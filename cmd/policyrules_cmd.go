package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sigs.k8s.io/yaml"
	"sort"
	"strings"

	"github.com/alcideio/rbac-tool/pkg/kube"
	"github.com/alcideio/rbac-tool/pkg/rbac"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func NewCommandPolicyRules() *cobra.Command {

	clusterContext := ""
	regex := ""
	inverse := false
	output := "table"
	// Support overrides
	cmd := &cobra.Command{
		Use:     "policy-rules",
		Aliases: []string{"rules", "rule", "policy", "pr"},
		Short:   "RBAC List Policy Rules For subject (user/group/serviceaccount) name",
		Long: `
List Kubernetes RBAC policy rules for a given User/ServiceAccount/Group

Examples:

# Search All Service Accounts
rbac-tool policy-rules -e '.*'

# Search All Service Accounts that contain myname
rbac-tool policy-rules -e '.*myname.*'

# Lookup System Accounts (all accounts that start with system: )
rbac-tool policy-rules -e '^system:.*'

# Lookup all accounts that DO NOT start with system: )
rbac-tool policy-rules -ne '^system:.*'

`,
		Hidden: false,
		RunE: func(c *cobra.Command, args []string) error {
			var re *regexp.Regexp
			var err error

			if regex != "" {
				re, err = regexp.Compile(regex)
			} else {
				if len(args) != 1 {
					re, err = regexp.Compile(fmt.Sprintf(`.*`))
				} else {
					re, err = regexp.Compile(fmt.Sprintf(`(?mi)%v`, args[0]))
				}
			}

			if err != nil {
				return err
			}

			client, err := kube.NewClient(clusterContext)
			if err != nil {
				return fmt.Errorf("Failed to create kubernetes client - %v", err)
			}

			perms, err := rbac.NewPermissionsFromCluster(client)
			if err != nil {
				return err
			}

			policies := rbac.NewSubjectPermissions(perms)
			filteredPolicies := []rbac.SubjectPermissions{}
			for _, policy := range policies {
				match := re.MatchString(policy.Subject.Name)

				//  match    inverse
				//  -----------------
				//  true     true   --> skip
				//  true     false  --> keep
				//  false    true   --> keep
				//  false    false  --> skip
				if match {
					if inverse {
						continue
					}
				} else {
					if !inverse {
						continue
					}
				}

				filteredPolicies = append(filteredPolicies, policy)
			}

			rows := [][]string{}

			for _, p := range filteredPolicies {
				name := p.Subject.Name
				kind := p.Subject.Kind
				for namespace, rules := range p.Rules {
					if namespace == "" {
						namespace = "*"
					}

					for _, rule := range rules {
						//Normalize the strings
						rbac.ReplaceToCore(rule.APIGroups)
						rbac.ReplaceToWildCard(rule.Resources)
						rbac.ReplaceToWildCard(rule.ResourceNames)
						rbac.ReplaceToWildCard(rule.Verbs)
						rbac.ReplaceToWildCard(rule.NonResourceURLs)

						sort.Strings(rule.APIGroups)
						sort.Strings(rule.Resources)
						sort.Strings(rule.ResourceNames)
						sort.Strings(rule.Verbs)
						sort.Strings(rule.NonResourceURLs)

						apigroups := strings.Join(rule.APIGroups, ",")
						if len(rule.APIGroups) == 0 {
							apigroups = "-"
						}

						resources := strings.Join(rule.Resources, ",")
						if len(rule.Resources) == 0 {
							resources = "-"
						}

						resourceNames := strings.Join(rule.ResourceNames, ",")
						if len(rule.ResourceNames) == 0 {
							if len(rule.APIGroups) == 0 {
								resourceNames = "-"
							} else {
								resourceNames = "*"
							}
						}

						nonResourceURLs := strings.Join(rule.NonResourceURLs, ",")
						if len(rule.NonResourceURLs) == 0 {
							nonResourceURLs = "-"
						}

						row := []string{
							kind,
							name,
							strings.Join(rule.Verbs, ","),
							namespace,
							apigroups,
							resources,
							resourceNames,
							nonResourceURLs,
						}
						rows = append(rows, row)
					}
				}
			}

			switch output {
			case "table":
				sort.Slice(rows, func(i, j int) bool {
					if strings.Compare(rows[i][0], rows[j][0]) == 0 {
						return (strings.Compare(rows[i][1], rows[j][1]) < 0)
					}

					return (strings.Compare(rows[i][0], rows[j][0]) < 0)
				})

				table := tablewriter.NewWriter(os.Stdout)
				table.SetHeader([]string{"TYPE", "SUBJECT", "VERBS", "NAMESPACE", "API GROUP", "KIND", "NAMES", "NonResourceURI"})
				table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
				table.SetBorder(false)
				table.SetAlignment(tablewriter.ALIGN_LEFT)
				//table.SetAutoMergeCells(true)

				table.AppendBulk(rows)
				table.Render()

				return nil
			case "yaml":
				policies := rbac.NewSubjectPermissionsList(filteredPolicies)
				data, err := yaml.Marshal(&policies)
				if err != nil {
					return fmt.Errorf("Processing error - %v", err)
				}
				fmt.Println(string(data))
				return nil

			case "json":
				policies := rbac.NewSubjectPermissionsList(filteredPolicies)
				data, err := json.Marshal(&policies)
				if err != nil {
					return fmt.Errorf("Processing error - %v", err)
				}

				fmt.Println(string(data))
				return nil
			default:
				return fmt.Errorf("Unsupported output format")
			}
		},
	}

	/**
	jmespath
	[? contains(@.allowedTo[].verbs[], 'get')] | [? contains(@.allowedTo[].apiGroups[], 'core')]
	*/

	flags := cmd.Flags()
	flags.StringVar(&clusterContext, "cluster-context", "", "Cluster Context .use 'kubectl config get-contexts' to list available contexts")
	flags.StringVarP(&output, "output", "o", "table", "Output type: table | json | yaml")

	flags.StringVarP(&regex, "regex", "e", "", "Specify whether run the lookup using a regex match")
	flags.BoolVarP(&inverse, "not", "n", false, "Inverse the regex matching. Use to search for users that do not match '^system:.*'")
	return cmd
}
