package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

// importedExists refuses an import by id that names nothing, in words.
//
// Left to Read, a missing object is removed from the state like any other
// that has gone — and after an import the framework answers that with
// "Missing Resource Identity After Read", which reads as a provider bug and
// names neither the id nor the thing. An id mistyped in an `import` block is
// the ordinary way to get here.
func importedExists(ctx context.Context, diags *diag.Diagnostics, what, id string, get func(context.Context, string) error) bool {
	err := get(ctx, id)
	if nuclock.IsNotFound(err) {
		diags.AddError("No such "+what, "nuclock has no "+what+" with id "+id+" — check the id in the import.")
		return false
	}
	if err != nil {
		apiError(diags, "look the "+what+" up", err)
		return false
	}
	return true
}
