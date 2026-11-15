package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoytest"
)

// These tests drive real Terraform (plan/apply/refresh/import/destroy)
// against an in-memory fake of the Convoy API, so they run without a Convoy
// instance or credentials. Set TF_ACC_TERRAFORM_PATH to use a local binary;
// otherwise terraform-plugin-testing downloads one.

var testProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"convoy": providerserver.NewProtocol6WithError(New("test")()),
	"echo":   echoprovider.NewProviderServer(),
}

const (
	testOrgID      = "org_test"
	testOutgoingID = "proj_outgoing"
	testIncomingID = "proj_incoming"
)

// newFakeConvoy starts a fake server seeded with one outgoing and one
// incoming project.
func newFakeConvoy(t *testing.T) *convoytest.Server {
	t.Helper()
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" && os.Getenv("TF_ACC_TERRAFORM_VERSION") == "" {
		t.Log("TF_ACC_TERRAFORM_PATH not set; terraform-plugin-testing will look up or download Terraform")
	}
	s := convoytest.NewServer()
	t.Cleanup(s.Close)
	for id, typ := range map[string]string{testOutgoingID: "outgoing", testIncomingID: "incoming"} {
		s.Put("projects", id, map[string]any{"uid": id, "name": id, "type": typ, "organisation_id": testOrgID, "config": map[string]any{}})
	}
	return s
}

func providerConfig(s *convoytest.Server) string {
	return fmt.Sprintf(`
provider "convoy" {
  api_key         = %q
  base_url        = %q
  project_id      = %q
  organisation_id = %q
  max_retries     = 0
}
`, convoytest.APIKey, s.BaseURL(), testOutgoingID, testOrgID)
}
