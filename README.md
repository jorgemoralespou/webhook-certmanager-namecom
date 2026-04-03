<p align="center">
  <img src="https://raw.githubusercontent.com/cert-manager/cert-manager/d53c0b9270f8cd90d908460d69502694e1838f5f/logo/logo-small.png" height="256" width="256" alt="cert-manager project logo" />
</p>

# cert-manager webhook for name.com

A [cert-manager](https://cert-manager.io) ACME DNS01 webhook solver for [name.com](https://www.name.com). It enables automatic TLS certificate issuance via Let's Encrypt (or any ACME-compatible CA) for domains managed by name.com.

## Prerequisites

- Kubernetes 1.21+
- [cert-manager](https://cert-manager.io/docs/installation/) v1.0.0+
- A [name.com](https://www.name.com) account with API access enabled
- A name.com API token ([generate one here](https://www.name.com/account/settings/api))

## Installation

The chart is published to two registries — use whichever suits your workflow.

### Option A: HTTP Helm repository (gh-pages)

```bash
helm repo add webhook-namecom https://jorgemoral.es/webhook-certmanager-namecom
helm repo update
```

Install the latest stable release:

```bash
helm install cert-manager-webhook-namecom webhook-namecom/cert-manager-webhook-namecom \
  --namespace cert-manager \
  --set groupName=acme.namecom.io
```

Install a specific version:

```bash
helm install cert-manager-webhook-namecom webhook-namecom/cert-manager-webhook-namecom \
  --namespace cert-manager \
  --set groupName=acme.namecom.io \
  --version 1.2.3
```

Install the latest development build (published on every commit to `main`):

```bash
helm install cert-manager-webhook-namecom webhook-namecom/cert-manager-webhook-namecom \
  --namespace cert-manager \
  --set groupName=acme.namecom.io \
  --devel
```

### Option B: OCI registry (GHCR)

No `helm repo add` needed — reference the chart directly:

```bash
helm install cert-manager-webhook-namecom \
  oci://ghcr.io/jorgemoralespou/charts/cert-manager-webhook-namecom \
  --namespace cert-manager \
  --set groupName=acme.namecom.io
```

Install a specific version:

```bash
helm install cert-manager-webhook-namecom \
  oci://ghcr.io/jorgemoralespou/charts/cert-manager-webhook-namecom \
  --namespace cert-manager \
  --set groupName=acme.namecom.io \
  --version 1.2.3
```

Install the latest development build:

```bash
helm install cert-manager-webhook-namecom \
  oci://ghcr.io/jorgemoralespou/charts/cert-manager-webhook-namecom \
  --namespace cert-manager \
  --set groupName=acme.namecom.io \
  --version 0.1.0-dev.5   # replace with the desired dev build number
```

### Credentials in a non-default namespace

By default the webhook can only read Secrets from the `cert-manager` namespace. If your credentials Secret lives elsewhere (e.g. cert-manager's `clusterResourceNamespace`), pass the namespace via `secretNamespaces`:

```bash
helm install cert-manager-webhook-namecom ... \
  --set "secretNamespaces={cert-manager,educates-secrets}"
```

The `groupName` must be a unique domain identifier you control. It is baked into the webhook and must match the value referenced in each Issuer configuration.

## Configuration

### 1. Create the API token Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: namecom-credentials
  namespace: cert-manager   # same namespace as the Issuer
type: Opaque
stringData:
  api-token: <your-name.com-api-token>
```

### 2. Create a ClusterIssuer or Issuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.namecom.io   # must match the value set during install
            solverName: namecom
            config:
              username: your-namecom-username
              apiTokenSecretRef:
                name: namecom-credentials
                key: api-token
```

### 3. Request a certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-tls
  namespace: default
spec:
  secretName: example-tls
  dnsNames:
    - example.com
    - "*.example.com"
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
```

## How it works

When cert-manager needs to validate domain ownership for a certificate request, it calls this webhook which:

1. **Present**: Creates a `_acme-challenge.<domain>` TXT record in name.com via the [name.com API](https://docs.name.com/).
2. **CleanUp**: Removes the TXT record once validation is complete, matching both the record name and the challenge key to safely handle concurrent challenges.

Authentication uses HTTP Basic Auth (`username:api-token`) against `https://api.name.com/core/v1`.

## Development

### Running the test suite

The webhook includes a conformance test suite using cert-manager's ACME test framework:

```bash
# Run basic tests using the in-memory mock DNS solver (no credentials needed)
make test

# Run conformance tests against a real name.com zone
TEST_ZONE_NAME=yourdomain.com. make test
```

To run against a real zone, update `main_test.go` to use `namecomDNSProviderSolver` (see the commented-out fixture) and create `testdata/my-custom-solver/config.json`:

```json
{
  "username": "your-namecom-username",
  "apiTokenSecretRef": {
    "name": "namecom-credentials",
    "key": "api-token"
  }
}
```

### Building the Docker image

```bash
make build IMAGE_NAME=ghcr.io/jorgemoralespou/webhook-certmanager-namecom IMAGE_TAG=dev
```

### Rendering the Helm chart

```bash
make rendered-manifest.yaml
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
# webhook-certmanager-namecom
