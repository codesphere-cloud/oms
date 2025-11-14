// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bufio"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Prompter", func() {
	Describe("NewPrompter", func() {
		It("creates a non-interactive prompter", func() {
			p := NewPrompter(false)
			Expect(p).NotTo(BeNil())
			Expect(p.interactive).To(BeFalse())
			Expect(p.reader).NotTo(BeNil())
		})

		It("creates an interactive prompter", func() {
			p := NewPrompter(true)
			Expect(p).NotTo(BeNil())
			Expect(p.interactive).To(BeTrue())
			Expect(p.reader).NotTo(BeNil())
		})
	})

	Describe("String", func() {
		Context("non-interactive mode", func() {
			It("returns default value without prompting", func() {
				p := NewPrompter(false)
				result := p.String("Enter value", "default")
				Expect(result).To(Equal("default"))
			})

			It("returns empty string when no default", func() {
				p := NewPrompter(false)
				result := p.String("Enter value", "")
				Expect(result).To(Equal(""))
			})
		})

		Context("interactive mode", func() {
			It("returns user input when provided", func() {
				input := "user-value\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.String("Enter value", "default")
				Expect(result).To(Equal("user-value"))
			})

			It("returns default when input is empty", func() {
				input := "\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.String("Enter value", "default")
				Expect(result).To(Equal("default"))
			})

			It("trims whitespace from input", func() {
				input := "  value with spaces  \n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.String("Enter value", "default")
				Expect(result).To(Equal("value with spaces"))
			})
		})
	})

	Describe("Int", func() {
		Context("non-interactive mode", func() {
			It("returns default value without prompting", func() {
				p := NewPrompter(false)
				result := p.Int("Enter number", 42)
				Expect(result).To(Equal(42))
			})
		})

		Context("interactive mode", func() {
			It("returns parsed integer when valid input provided", func() {
				input := "123\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Int("Enter number", 42)
				Expect(result).To(Equal(123))
			})

			It("returns default when input is empty", func() {
				input := "\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Int("Enter number", 42)
				Expect(result).To(Equal(42))
			})

			It("returns default when input is invalid", func() {
				input := "not-a-number\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Int("Enter number", 42)
				Expect(result).To(Equal(42))
			})

			It("handles negative numbers", func() {
				input := "-100\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Int("Enter number", 0)
				Expect(result).To(Equal(-100))
			})
		})
	})

	Describe("StringSlice", func() {
		Context("non-interactive mode", func() {
			It("returns default value without prompting", func() {
				p := NewPrompter(false)
				defaultVal := []string{"one", "two", "three"}
				result := p.StringSlice("Enter values", defaultVal)
				Expect(result).To(Equal(defaultVal))
			})

			It("returns empty slice when no default", func() {
				p := NewPrompter(false)
				result := p.StringSlice("Enter values", []string{})
				Expect(result).To(Equal([]string{}))
			})
		})

		Context("interactive mode", func() {
			It("parses comma-separated values", func() {
				input := "one, two, three\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.StringSlice("Enter values", []string{})
				Expect(result).To(Equal([]string{"one", "two", "three"}))
			})

			It("returns default when input is empty", func() {
				input := "\n"
				defaultVal := []string{"default1", "default2"}
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.StringSlice("Enter values", defaultVal)
				Expect(result).To(Equal(defaultVal))
			})

			It("trims whitespace from each value", func() {
				input := "  one  ,  two  ,  three  \n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.StringSlice("Enter values", []string{})
				Expect(result).To(Equal([]string{"one", "two", "three"}))
			})

			It("handles single value", func() {
				input := "single\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.StringSlice("Enter values", []string{})
				Expect(result).To(Equal([]string{"single"}))
			})

			It("filters out empty values", func() {
				input := "one, , two, , three\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.StringSlice("Enter values", []string{})
				Expect(result).To(Equal([]string{"one", "two", "three"}))
			})
		})
	})

	Describe("Bool", func() {
		Context("non-interactive mode", func() {
			It("returns default true without prompting", func() {
				p := NewPrompter(false)
				result := p.Bool("Enable feature", true)
				Expect(result).To(BeTrue())
			})

			It("returns default false without prompting", func() {
				p := NewPrompter(false)
				result := p.Bool("Enable feature", false)
				Expect(result).To(BeFalse())
			})
		})

		Context("interactive mode", func() {
			DescribeTable("boolean parsing",
				func(input string, expected bool) {
					p := &Prompter{
						reader:      bufio.NewReader(strings.NewReader(input + "\n")),
						interactive: true,
					}
					result := p.Bool("Enable feature", false)
					Expect(result).To(Equal(expected))
				},
				Entry("'y' returns true", "y", true),
				Entry("'Y' returns true", "Y", true),
				Entry("'yes' returns true", "yes", true),
				Entry("'YES' returns true", "YES", true),
				Entry("'n' returns false", "n", false),
				Entry("'N' returns false", "N", false),
				Entry("'no' returns false", "no", false),
				Entry("'NO' returns false", "NO", false),
				Entry("invalid input returns false", "maybe", false),
			)

			It("returns default when input is empty", func() {
				input := "\n"
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Bool("Enable feature", true)
				Expect(result).To(BeTrue())
			})
		})
	})

	Describe("Choice", func() {
		Context("non-interactive mode", func() {
			It("returns default value without prompting", func() {
				p := NewPrompter(false)
				choices := []string{"option1", "option2", "option3"}
				result := p.Choice("Select option", choices, "option2")
				Expect(result).To(Equal("option2"))
			})
		})

		Context("interactive mode", func() {
			It("returns matching choice case-insensitively", func() {
				input := "OPTION2\n"
				choices := []string{"option1", "option2", "option3"}
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Choice("Select option", choices, "option1")
				Expect(result).To(Equal("option2"))
			})

			It("returns default when input is empty", func() {
				input := "\n"
				choices := []string{"option1", "option2", "option3"}
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Choice("Select option", choices, "option2")
				Expect(result).To(Equal("option2"))
			})

			It("returns default when input is invalid", func() {
				input := "invalid-option\n"
				choices := []string{"option1", "option2", "option3"}
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Choice("Select option", choices, "option1")
				Expect(result).To(Equal("option1"))
			})

			It("handles exact match", func() {
				input := "option2\n"
				choices := []string{"option1", "option2", "option3"}
				p := &Prompter{
					reader:      bufio.NewReader(strings.NewReader(input)),
					interactive: true,
				}
				result := p.Choice("Select option", choices, "option1")
				Expect(result).To(Equal("option2"))
			})
		})
	})
})
