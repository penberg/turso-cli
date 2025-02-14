package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/chiselstrike/iku-turso-cli/internal/settings"
	"github.com/chiselstrike/iku-turso-cli/internal/turso"
	"github.com/spf13/cobra"
)

func init() {
	dbCmd.AddCommand(showCmd)
	showCmd.Flags().BoolVar(&showUrlFlag, "url", false, "Show URL for the database HTTP API.")
	showCmd.Flags().BoolVar(&showBasicAuthFlag, "basic-auth", false, "Show basic authentication in the URL.")
	showCmd.Flags().StringVar(&showInstanceUrlFlag, "instance-url", "", "Show URL for the HTTP API of a selected instance of a database. Instance is selected by instance name.")
	showCmd.RegisterFlagCompletionFunc("instance-url", completeInstanceName)
	showCmd.RegisterFlagCompletionFunc("instance-ws-url", completeInstanceName)
}

var showCmd = &cobra.Command{
	Use:               "show database_name",
	Short:             "Show information from a database.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: dbNameArg,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		client, err := createTursoClient()
		if err != nil {
			return err
		}
		db, err := getDatabase(client, args[0])
		if err != nil {
			return err
		}

		config, err := settings.ReadSettings()
		if err != nil {
			return err
		}

		if showUrlFlag {
			fmt.Println(getDatabaseUrl(config, &db, showBasicAuthFlag))
			return nil
		}

		if showHttpUrlFlag {
			fmt.Println(getDatabaseHttpUrl(config, &db))
			return nil
		}

		instances, err := client.Instances.List(db.Name)
		if err != nil {
			return fmt.Errorf("could not get instances of database %s: %w", db.Name, err)
		}

		if showInstanceUrlFlag != "" {
			for _, instance := range instances {
				if instance.Name == showInstanceUrlFlag {
					fmt.Println(getInstanceUrl(config, &db, &instance))
					return nil
				}
			}
			return fmt.Errorf("instance %s was not found for database %s. List known instances using %s", turso.Emph(showInstanceUrlFlag), turso.Emph(db.Name), turso.Emph("turso db show "+db.Name))
		}

		regions := make([]string, len(db.Regions))
		copy(regions, db.Regions)
		sort.Strings(regions)

		fmt.Println("Name:          ", db.Name)
		fmt.Println("URL:           ", getDatabaseUrl(config, &db, false))
		fmt.Println("ID:            ", db.ID)
		fmt.Println("Locations:     ", strings.Join(regions, ", "))
		fmt.Println()

		versions := [](chan string){}
		urls := []string{}
		httpUrls := []string{}
		for idx, instance := range instances {
			urls = append(urls, getInstanceUrl(config, &db, &instance))
			httpUrls = append(httpUrls, getInstanceHttpUrl(config, &db, &instance))
			versions = append(versions, make(chan string, 1))
			go func(idx int) {
				versions[idx] <- fetchInstanceVersion(httpUrls[idx])
			}(idx)
		}

		data := [][]string{}
		for idx, instance := range instances {
			version := <-versions[idx]
			data = append(data, []string{instance.Name, instance.Type, instance.Region, version, urls[idx]})
		}

		fmt.Print("Database Instances:\n")
		printTable([]string{"Name", "Type", "Location", "Version", "URL"}, data)

		return nil
	},
}

func fetchInstanceVersion(baseUrl string) string {
	url, err := url.Parse(baseUrl)
	if err != nil {
		return fmt.Sprintf("fetch failed: %s", err)
	}
	url.Path = path.Join(url.Path, "/version")
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return fmt.Sprintf("fetch failed: %s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("fetch failed: %s", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return "0.3.1-"
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("fetch failed: %s", err)
	}
	return string(respBody)
}
