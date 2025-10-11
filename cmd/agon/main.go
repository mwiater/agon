// cmd/agon/main.go
package main

import (
	cmd "github.com/mwiater/agon/internal/cli"
)

// main starts the agon CLI application by delegating to the
// cobra root command defined in the agon package. It does not
// take any arguments and does not return a value.
func main() {
	cmd.Execute()
}
