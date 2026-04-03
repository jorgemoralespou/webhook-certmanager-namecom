package main

import (
	"os"
	"testing"

	acmetest "github.com/cert-manager/cert-manager/test/acme"

	"github.com/cert-manager/webhook-example/example"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
)

func TestRunsSuite(t *testing.T) {
	// To run the namecom solver against a real name.com account, set:
	//   TEST_ZONE_NAME=yourdomain.com. make test
	// and create testdata/my-custom-solver/config.json with your credentials reference.
	// Then replace the fixture below with:
	//
	//fixture := acmetest.NewFixture(&namecomDNSProviderSolver{},
	//	acmetest.SetResolvedZone(zone),
	//	acmetest.SetAllowAmbientCredentials(false),
	//	acmetest.SetManifestPath("testdata/my-custom-solver"),
	//	acmetest.SetBinariesPath("_test/kubebuilder/bin"),
	//)
	//fixture.RunConformance(t)

	// Basic conformance tests using the in-memory mock DNS solver.
	solver := example.New("59351")
	fixture := acmetest.NewFixture(solver,
		acmetest.SetResolvedZone("example.com."),
		acmetest.SetManifestPath("testdata/my-custom-solver"),
		acmetest.SetDNSServer("127.0.0.1:59351"),
		acmetest.SetUseAuthoritative(false),
	)
	fixture.RunBasic(t)
	fixture.RunExtended(t)
}
