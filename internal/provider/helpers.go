package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

// clientFrom takes the configured client out of provider data, or says why not.
func clientFrom(data any, diags *diag.Diagnostics) *nuclock.Client {
	if data == nil {
		return nil // Not configured yet: the framework calls Configure again.
	}
	client, ok := data.(*nuclock.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *nuclock.Client, got %T.", data))
		return nil
	}
	return client
}

// apiError reports a refusal with the API's own words. For `ability_missing`
// those words name the ability the key lacks, which is the whole of the fix.
func apiError(diags *diag.Diagnostics, doing string, err error) {
	diags.AddError("nuclock refused to "+doing, err.Error())
}

func stringSet(values []string) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	set, _ := types.SetValue(types.StringType, elements)
	return set
}

func setStrings(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(set.ElementsAs(ctx, &out, false)...)
	sort.Strings(out)
	return out
}

func optionalString(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

// orEmpty is a null-safe slice for a JSON body: `[]`, never `null`.
func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
