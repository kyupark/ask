package cmd

import (
	"fmt"
	"strings"

	"github.com/qm4/webai-cli/internal/provider"
)

// runModels prints the model catalog for a provider implementing ModelLister.
func runModels(p provider.Provider) error {
	ml, ok := p.(provider.ModelLister)
	if !ok {
		return fmt.Errorf("%s does not support listing models", p.Name())
	}

	catalog := ml.ListModels()

	fmt.Printf("%s — Available Models\n", strings.ToUpper(catalog.Provider))
	fmt.Println(strings.Repeat("─", 60))

	for _, m := range catalog.Models {
		defaultMark := "  "
		if m.Default {
			defaultMark = "* "
		}
		fmt.Printf("%s%-30s %s\n", defaultMark, m.ID, m.Name)
		if m.Description != "" {
			fmt.Printf("  %-30s %s\n", "", m.Description)
		}
		if len(m.Tags) > 0 {
			fmt.Printf("  %-30s [%s]\n", "", strings.Join(m.Tags, ", "))
		}
	}

	if len(catalog.Modes) > 0 {
		fmt.Println()
		fmt.Println("Modes:")
		for _, m := range catalog.Modes {
			defaultMark := "  "
			if m.Default {
				defaultMark = "* "
			}
			fmt.Printf("%s%-20s %s\n", defaultMark, m.ID, m.Description)
		}
	}

	if len(catalog.SearchFocus) > 0 {
		fmt.Println()
		fmt.Println("Search Focus:")
		for _, s := range catalog.SearchFocus {
			defaultMark := "  "
			if s.Default {
				defaultMark = "* "
			}
			fmt.Printf("%s%-20s %s\n", defaultMark, s.ID, s.Description)
		}
	}

	fmt.Println()
	fmt.Println("(* = default)")
	return nil
}