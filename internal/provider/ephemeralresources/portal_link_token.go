// Package ephemeralresources contains ephemeral resources: values that are
// produced on every run and never persisted to plan or state.
package ephemeralresources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var _ ephemeral.EphemeralResourceWithConfigure = &PortalLinkTokenEphemeralResource{}

func NewPortalLinkTokenEphemeralResource() ephemeral.EphemeralResource {
	return &PortalLinkTokenEphemeralResource{}
}

type PortalLinkTokenEphemeralResource struct{ data *shared.ProviderData }

type portalLinkTokenModel struct {
	ProjectID    types.String `tfsdk:"project_id"`
	PortalLinkID types.String `tfsdk:"portal_link_id"`
	Token        types.String `tfsdk:"token"`
}

func (e *PortalLinkTokenEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portal_link_token"
}

func (e *PortalLinkTokenEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Issues a fresh, short-lived access token for a portal link that uses the `refresh_token` auth type. " +
			"The token is never stored in the Terraform plan or state; pass it to write-only arguments or other ephemeral contexts. " +
			"Requires Terraform 1.10 or later.",
		Attributes: map[string]schema.Attribute{
			"project_id":     schema.StringAttribute{Optional: true, Computed: true, Description: "Project ID. Defaults to the provider's `project_id`."},
			"portal_link_id": schema.StringAttribute{Required: true, Description: "Portal link ID."},
			"token":          schema.StringAttribute{Computed: true, Sensitive: true, Description: "The new portal access token."},
		},
	}
}

func (e *PortalLinkTokenEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	e.data = shared.FromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (e *PortalLinkTokenEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var m portalLinkTokenModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := e.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	token, err := e.data.Client.RefreshPortalLinkToken(ctx, projectID, m.PortalLinkID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to refresh portal link token: %s", err))
		return
	}
	m.ProjectID = types.StringValue(projectID)
	m.Token = types.StringValue(token)
	resp.Diagnostics.Append(resp.Result.Set(ctx, &m)...)
}
