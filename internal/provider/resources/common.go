// Package resources contains the managed resources of the Convoy provider.
package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

// base implements Configure for every resource.
type base struct {
	data *shared.ProviderData
}

func (b *base) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	b.data = shared.FromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (b *base) client() *convoy.Client { return b.data.Client }

// ------------------------------------------------------- common attributes

func idAttribute(what string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:      true,
		Description:   fmt.Sprintf("Unique identifier (`uid`) of the %s.", what),
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func projectIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true,
		Computed: true,
		Description: "ID of the project the object belongs to. Defaults to the provider's `project_id`. " +
			"Changing this forces a new object to be created.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func timestampAttribute(desc string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, Description: desc,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
}

// stringOC is an optional+computed string that keeps its prior value when
// omitted from configuration.
func stringOC(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:      true,
		Computed:      true,
		Description:   desc,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// ---------------------------------------------------------- object helpers

// objectFrom converts a struct with tfsdk tags into an object value.
func objectFrom(ctx context.Context, attrTypes map[string]attr.Type, v any, diags *diag.Diagnostics) types.Object {
	obj, d := types.ObjectValueFrom(ctx, attrTypes, v)
	diags.Append(d...)
	return obj
}

// objectAs decodes a known object into target and reports whether it did.
func objectAs(ctx context.Context, o types.Object, target any, diags *diag.Diagnostics) bool {
	if o.IsNull() || o.IsUnknown() {
		return false
	}
	diags.Append(o.As(ctx, target, basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true})...)
	return !diags.HasError()
}

// objectAttrs returns the attribute types of a nested attribute's object.
func objectAttrs(a schema.SingleNestedAttribute) map[string]attr.Type {
	return a.GetType().(types.ObjectType).AttrTypes
}

// handleNotFound removes a resource from state when the API says it is gone.
func handleNotFound(ctx context.Context, err error, resp *resource.ReadResponse, what string) bool {
	if convoy.IsNotFound(err) {
		resp.Diagnostics.AddWarning(what+" not found", "The "+what+" no longer exists in Convoy and has been removed from the Terraform state.")
		resp.State.RemoveResource(ctx)
		return true
	}
	return false
}

func apiError(diags *diag.Diagnostics, action string, err error) {
	diags.AddError("Convoy API error", fmt.Sprintf("Unable to %s: %s", action, err))
}

// merge writes v into dst when refreshing from the API (Read/Import) or when
// dst is unknown (a computed value the plan could not know). During
// Create/Update, configured values are kept as planned so that features
// silently ignored by a Convoy edition surface as a diff on the next plan
// instead of an "inconsistent result" error.
func merge[T attr.Value](dst *T, v T, refresh bool) {
	if refresh || (*dst).IsUnknown() {
		*dst = v
	}
}

// mergeObject replaces unknown values inside a planned object (recursively)
// with the corresponding values of fresh, which was built from the API.
func mergeObject(planned, fresh types.Object) types.Object {
	if planned.IsUnknown() {
		return fresh
	}
	if planned.IsNull() || fresh.IsNull() || fresh.IsUnknown() {
		return planned
	}
	freshAttrs := fresh.Attributes()
	out := make(map[string]attr.Value, len(planned.Attributes()))
	for k, v := range planned.Attributes() {
		switch {
		case v.IsUnknown():
			out[k] = freshAttrs[k]
		default:
			if po, ok := v.(types.Object); ok {
				if fo, ok := freshAttrs[k].(types.Object); ok {
					out[k] = mergeObject(po, fo)
					continue
				}
			}
			out[k] = v
		}
	}
	return types.ObjectValueMust(planned.AttributeTypes(context.Background()), out)
}
