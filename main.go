package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

const namecomAPIBase = "https://api.name.com/core/v1"

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}
	cmd.RunWebhookServer(GroupName,
		&namecomDNSProviderSolver{},
	)
}

// namecomDNSProviderSolver implements the cert-manager webhook.Solver interface
// for the name.com DNS provider.
type namecomDNSProviderSolver struct {
	client *kubernetes.Clientset
}

// namecomDNSProviderConfig holds the per-issuer configuration for the solver.
// It is decoded from the `issuer.spec.acme.dns01.providers.webhook.config` field.
type namecomDNSProviderConfig struct {
	// Username is the name.com account username.
	Username string `json:"username"`
	// APITokenSecretRef references the Kubernetes Secret containing the name.com API token.
	APITokenSecretRef corev1.SecretKeySelector `json:"apiTokenSecretRef"`
}

// namecomRecord represents a DNS record in the name.com API.
type namecomRecord struct {
	ID         int32  `json:"id,omitempty"`
	DomainName string `json:"domainName,omitempty"`
	Host       string `json:"host"`
	Type       string `json:"type"`
	Answer     string `json:"answer"`
	TTL        uint32 `json:"ttl"`
}

type namecomListRecordsResponse struct {
	Records []*namecomRecord `json:"records"`
}

// Name returns the solver identifier used in the ACME Issuer configuration.
func (c *namecomDNSProviderSolver) Name() string {
	return "namecom"
}

// Present creates the TXT record required for the DNS01 ACME challenge.
func (c *namecomDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	apiToken, err := c.getAPIToken(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	domain := extractDomainName(ch.ResolvedZone)
	host := extractRecordName(ch.ResolvedFQDN, ch.ResolvedZone)

	_, err = c.createRecord(cfg.Username, apiToken, domain, &namecomRecord{
		Host:   host,
		Type:   "TXT",
		Answer: ch.Key,
		TTL:    300,
	})
	return err
}

// CleanUp removes the TXT record that was created for the DNS01 ACME challenge.
// It lists all records and deletes the one matching both the host and the challenge key,
// so that concurrent challenges for the same domain are handled correctly.
func (c *namecomDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	apiToken, err := c.getAPIToken(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	domain := extractDomainName(ch.ResolvedZone)
	host := extractRecordName(ch.ResolvedFQDN, ch.ResolvedZone)

	records, err := c.listRecords(cfg.Username, apiToken, domain)
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Type == "TXT" && record.Host == host && record.Answer == ch.Key {
			return c.deleteRecord(cfg.Username, apiToken, domain, record.ID)
		}
	}

	// Record not found — already cleaned up or never created; not an error.
	return nil
}

// Initialize sets up the Kubernetes clientset used to fetch Secret resources.
func (c *namecomDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}
	c.client = cl
	return nil
}

// getAPIToken reads the API token from the Kubernetes Secret referenced in cfg.
func (c *namecomDNSProviderSolver) getAPIToken(cfg namecomDNSProviderConfig, namespace string) (string, error) {
	secret, err := c.client.CoreV1().Secrets(namespace).Get(
		context.Background(),
		cfg.APITokenSecretRef.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("error fetching secret %q: %v", cfg.APITokenSecretRef.Name, err)
	}
	token, ok := secret.Data[cfg.APITokenSecretRef.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %q", cfg.APITokenSecretRef.Key, cfg.APITokenSecretRef.Name)
	}
	return string(token), nil
}

// createRecord creates a DNS record via the name.com API.
func (c *namecomDNSProviderSolver) createRecord(username, apiToken, domain string, record *namecomRecord) (*namecomRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/domains/%s/records", namecomAPIBase, domain)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("name.com API error creating record (status %d): %s", resp.StatusCode, string(respBody))
	}

	var created namecomRecord
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	return &created, nil
}

// listRecords retrieves all DNS records for a domain from the name.com API.
func (c *namecomDNSProviderSolver) listRecords(username, apiToken, domain string) ([]*namecomRecord, error) {
	url := fmt.Sprintf("%s/domains/%s/records", namecomAPIBase, domain)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, apiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("name.com API error listing records (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result namecomListRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Records, nil
}

// deleteRecord removes a DNS record by ID via the name.com API.
func (c *namecomDNSProviderSolver) deleteRecord(username, apiToken, domain string, id int32) error {
	url := fmt.Sprintf("%s/domains/%s/records/%d", namecomAPIBase, domain, id)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, apiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("name.com API error deleting record %d (status %d): %s", id, resp.StatusCode, string(respBody))
	}
	return nil
}

// extractRecordName returns the relative host by stripping the zone suffix from the FQDN.
// e.g. "_acme-challenge.example.com." with zone "example.com." -> "_acme-challenge"
func extractRecordName(fqdn, zone string) string {
	if strings.HasSuffix(fqdn, "."+zone) {
		return strings.TrimSuffix(fqdn, "."+zone)
	}
	// Apex case: fqdn == zone
	return strings.TrimSuffix(strings.TrimSuffix(fqdn, zone), ".")
}

// extractDomainName strips the trailing dot from a DNS zone name.
// e.g. "example.com." -> "example.com"
func extractDomainName(zone string) string {
	return strings.TrimSuffix(zone, ".")
}

// loadConfig decodes the per-challenge JSON configuration into the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (namecomDNSProviderConfig, error) {
	cfg := namecomDNSProviderConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}
	return cfg, nil
}
