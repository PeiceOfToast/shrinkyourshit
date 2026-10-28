// This is a stub go.mod. It exists only so the Go toolchain treats web/ as a
// separate module and never descends into web/node_modules, where vendored
// Go files would otherwise be picked up by ./... build, vet and test
// patterns. The web app is built with npm (see STACK.md); nothing here is
// compiled by the root module.
module github.com/PeiceOfToast/shrinkyourshit/web

go 1.23
