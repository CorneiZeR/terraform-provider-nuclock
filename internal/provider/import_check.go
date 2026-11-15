package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

// importedExists refuses an import by id that names nothing, or names
// something put away, in words.
//
// Left to Read, a missing object is removed from the state like any other
// that has gone — and after an import the framework answers that with
// "Missing Resource Identity After Read", which reads as a provider bug and
// names neither the id nor the thing. An id mistyped in an `import` block is
// the ordinary way to get here.
//
// `get` answers whether the object is live. One that is not — a blocked
// person, an archived project, a closed seat — is what a destroy leaves
// behind, and Read treats it as gone for the same reason; refusePutAway says
// what to do instead.
func importedExists(ctx context.Context, diags *diag.Diagnostics, what, putAway, id string, get func(context.Context, string) (bool, error)) bool {
	live, err := get(ctx, id)
	if nuclock.IsNotFound(err) {
		diags.AddError("No such "+what, "nuclock has no "+what+" with id "+id+" — check the id in the import.")
		return false
	}
	if err != nil {
		apiError(diags, "look the "+what+" up", err)
		return false
	}
	if !live {
		refusePutAway(diags, what, putAway, id)
		return false
	}
	return true
}

// refusePutAway refuses to import an object a destroy left behind.
//
// Such an object reads as gone, so an import of it would end in the
// framework's "Missing Resource Identity After Read". It needs no import at
// all: declared without one, Create finds it and brings it back with its
// history — the person let back in, the project restored, the seat reopened.
func refusePutAway(diags *diag.Diagnostics, what, putAway, ref string) {
	diags.AddError("The "+what+" is "+putAway,
		"nuclock's "+what+" "+ref+" is "+putAway+", which this provider reads as gone. "+
			"Remove the import and keep the resource: creating it brings the "+what+" back, with its history.")
}
