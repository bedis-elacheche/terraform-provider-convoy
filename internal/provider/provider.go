// Package provider implements the Convoy Terraform provider.
package provider

import (
	"context"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/actions"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/datasources"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/ephemeralresources"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/resources"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ provider.Provider                       = &ConvoyProvider{}
	_ provider.ProviderWithEphemeralResources = &ConvoyProvider{}
	_ provider.ProviderWithActions            = &ConvoyProvider{}
)

// Environment variables read by the provider.
const (
	EnvAPIKey         = "CONVOY_API_KEY"
	EnvBaseURL        = "CONVOY_BASE_URL"
	EnvProjectID      = "CONVOY_PROJECT_ID"
	EnvOrganisationID = "CONVOY_ORGANISATION_ID"
	EnvMaxRetries     = "CONVOY_MAX_RETRIES"
)

type ConvoyProvider struct {
	version string
}

type ConvoyProviderModel struct {
	APIKey         types.String `tfsdk:"api_key"`
	BaseURL        types.String `tfsdk:"base_url"`
	ProjectID      types.String `tfsdk:"project_id"`
	OrganisationID types.String `tfsdk:"organisation_id"`
	MaxRetries     types.Int64  `tfsdk:"max_retries"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &ConvoyProvider{version: version} }
}

func (p *ConvoyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "convoy"
	resp.Version = p.version
}

func (p *ConvoyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage [Convoy](https://www.getconvoy.io) webhooks gateway resources (projects, endpoints, sources, subscriptions, event types and portal links) with Terraform.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Convoy API key. A project API key can manage resources of its own project; a personal API key is required to manage `convoy_project`. Can also be set via the `" + EnvAPIKey + "` environment variable.",
			},
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the Convoy API, including the `/api` suffix. Defaults to `" + convoy.DefaultBaseURL + "` (Convoy Cloud US). Use `https://eu.getconvoy.cloud/api` for the EU region or your own host for self-hosted Convoy. Can also be set via the `" + EnvBaseURL + "` environment variable.",
			},
			"project_id": schema.StringAttribute{
				Optional:    true,
				Description: "Default project for project-scoped resources that do not set `project_id` themselves. Can also be set via the `" + EnvProjectID + "` environment variable.",
			},
			"organisation_id": schema.StringAttribute{
				Optional:    true,
				Description: "Default organisation used by `convoy_project`. Can also be set via the `" + EnvOrganisationID + "` environment variable.",
			},
			"max_retries": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum number of retries for rate-limited (HTTP 429) and transient gateway errors. Defaults to `3`. Can also be set via the `" + EnvMaxRetries + "` environment variable.",
			},
		},
	}
}

func (p *ConvoyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config ConvoyProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for attr, v := range map[string]types.String{"api_key": config.APIKey, "base_url": config.BaseURL, "project_id": config.ProjectID, "organisation_id": config.OrganisationID} {
		if v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(attr), "Unknown provider configuration value",
				"The provider cannot be configured with a value that is only known after apply. Set it statically or via environment variables.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := valueOrEnv(config.APIKey, EnvAPIKey)
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing API key",
			"Set the api_key provider attribute or the "+EnvAPIKey+" environment variable.")
		return
	}
	baseURL := valueOrEnv(config.BaseURL, EnvBaseURL)

	retries := 3
	if !config.MaxRetries.IsNull() && !config.MaxRetries.IsUnknown() {
		retries = int(config.MaxRetries.ValueInt64())
	} else if v := os.Getenv(EnvMaxRetries); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			resp.Diagnostics.AddError("Invalid "+EnvMaxRetries, err.Error())
			return
		}
		retries = n
	}
	if retries < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("max_retries"), "Invalid max_retries", "max_retries must be >= 0")
		return
	}

	client, err := convoy.NewClient(baseURL, apiKey,
		convoy.WithUserAgent("terraform-provider-convoy/"+p.version+" (+https://registry.terraform.io/providers/bedis-elacheche/convoy)"),
		convoy.WithMaxRetries(retries),
	)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("base_url"), "Invalid provider configuration", err.Error())
		return
	}

	data := &shared.ProviderData{
		Client:         client,
		ProjectID:      valueOrEnv(config.ProjectID, EnvProjectID),
		OrganisationID: valueOrEnv(config.OrganisationID, EnvOrganisationID),
	}
	resp.DataSourceData = data
	resp.ResourceData = data
	resp.EphemeralResourceData = data
	resp.ActionData = data
}

func valueOrEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func (p *ConvoyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewProjectResource,
		resources.NewEndpointResource,
		resources.NewSourceResource,
		resources.NewSubscriptionResource,
		resources.NewSubscriptionFilterResource,
		resources.NewEventTypeResource,
		resources.NewPortalLinkResource,
	}
}

func (p *ConvoyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewProjectDataSource,
		datasources.NewEndpointDataSource,
		datasources.NewSourceDataSource,
		datasources.NewEventTypesDataSource,
	}
}

func (p *ConvoyProvider) EphemeralResources(_ context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		ephemeralresources.NewPortalLinkTokenEphemeralResource,
	}
}

func (p *ConvoyProvider) Actions(_ context.Context) []func() action.Action {
	return []func() action.Action{
		actions.NewSendEventAction,
		actions.NewEndpointSecretRotationAction,
		actions.NewEventReplayAction,
	}
}
