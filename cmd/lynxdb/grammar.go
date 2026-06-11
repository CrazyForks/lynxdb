package main

import (
	"embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed grammar_data/*
var grammarFS embed.FS

func init() {
	rootCmd.AddCommand(newGrammarCmd())
}

func newGrammarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "grammar [ebnf|cookbook]",
		Short: "Print LynxFlow grammar documentation for LLM integration",
		Long: `Prints machine-readable LynxFlow grammar documentation designed for LLM-based
natural-language-to-LynxFlow translation systems.

Subcommands:
  ebnf       Complete EBNF grammar specification
  cookbook    Prompt cookbook with system prompt, few-shot, and error correction patterns`,
		Example: `  lynxdb grammar ebnf                          Print EBNF grammar
  lynxdb grammar cookbook > prompts.md         Export cookbook`,
		Args: cobra.ExactArgs(1),
		RunE: runGrammar,
	}
}

func runGrammar(_ *cobra.Command, args []string) error {
	var filename string

	switch args[0] {
	case "ebnf":
		filename = "grammar_data/lynxflow.ebnf"
	case "cookbook":
		filename = "grammar_data/llm-cookbook.md"
	default:
		return fmt.Errorf("unknown grammar subcommand %q. Use: ebnf, cookbook", args[0])
	}

	data, err := grammarFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read embedded grammar: %w", err)
	}

	fmt.Print(string(data))

	return nil
}
