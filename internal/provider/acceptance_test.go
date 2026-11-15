package provider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccRealConvoy runs against a real Convoy instance. It is skipped unless
// TF_ACC=1 and the following are set:
//
//	CONVOY_API_KEY     project (or personal) API key
//	CONVOY_PROJECT_ID  an existing *outgoing* project used for the test
//	CONVOY_BASE_URL    optional, defaults to Convoy Cloud US
//
// It creates and destroys an endpoint, a subscription and a portal link.
// Event types cannot be deleted in Convoy, so none are created here.
func TestAccRealConvoy(t *testing.T) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("CONVOY_API_KEY") == "" || os.Getenv("CONVOY_PROJECT_ID") == "" {
		t.Skip("set TF_ACC, CONVOY_API_KEY and CONVOY_PROJECT_ID to run acceptance tests against Convoy")
	}
	suffix := fmt.Sprintf("%d", time.Now().Unix())
	cfg := func(timeout int) string {
		return fmt.Sprintf(`
provider "convoy" {}

resource "convoy_endpoint" "acc" {
  name         = "tf-acc-%[1]s"
  url          = "https://example.com/tf-acc/%[1]s"
  owner_id     = "tf-acc-%[1]s"
  http_timeout = %[2]d
}

resource "convoy_subscription" "acc" {
  name        = "tf-acc-%[1]s"
  endpoint_id = convoy_endpoint.acc.id
  filter_config = {
    event_types = ["tf.acc"]
    filter = { body = jsonencode({ data = { ok = true } }) }
  }
}

resource "convoy_portal_link" "acc" {
  name     = "tf-acc-%[1]s"
  owner_id = convoy_endpoint.acc.owner_id
}
`, suffix, timeout)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"convoy": providerserver.NewProtocol6WithError(New("acc")()),
		},
		Steps: []resource.TestStep{
			{Config: cfg(10)},
			{Config: cfg(20)},
			{ResourceName: "convoy_endpoint.acc", ImportState: true, ImportStateVerify: true},
			{ResourceName: "convoy_subscription.acc", ImportState: true, ImportStateVerify: true},
		},
	})
}
