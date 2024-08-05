package main

import (
	"fmt"
	"github.com/Ja7ad/meilishell/util"
	shell "github.com/brianstrauch/cobra-shell"
	"github.com/c-bata/go-prompt"
	"github.com/fatih/color"
	"github.com/inancgumus/screen"
	"github.com/meilisearch/meilisearch-go"
	"github.com/spf13/cobra"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
)

const header = `MeiliShell %s | https://github.com/Ja7ad/meilishell

- Server: Meilisearch v%s
- Time: %s
- Docs: https://www.meilisearch.com/docs/reference/api/overview
- Status: %s
`

const (
	major = 0
	minor = 1
	patch = 0
)

func version() string {
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch)
}

var (
	_prefix = "Meilishell@%s > "
	client  *meilisearch.Client
)

func main() {
	root := &cobra.Command{
		Use:   "meilishell",
		Short: "Meilisearch shell",
	}

	sh := shell.New(root, nil,
		prompt.OptionSuggestionBGColor(prompt.Black),
		prompt.OptionSuggestionTextColor(prompt.Green),
		prompt.OptionDescriptionBGColor(prompt.Black),
		prompt.OptionDescriptionTextColor(prompt.White),
		prompt.OptionLivePrefix(livePrefix),
	)

	h := sh.PersistentFlags().String("host", "http://localhost:7700", "set meilisearch host")
	k := sh.PersistentFlags().String("api-key", "", "set meilisearch api key or master key "+
		"(https://www.meilisearch.com/docs/reference/api/keys)")

	sh.PreRun = func(cmd *cobra.Command, args []string) {
		u, err := url.Parse(*h)
		if err != nil {
			log.Fatal(err)
		}

		client = meilisearch.NewClient(meilisearch.ClientConfig{
			Host:   u.String(),
			APIKey: *k,
		})

		if !client.IsHealthy() {
			color.Red("❌ Failed connect to Meilisearch, Host or API-Key is invalid")
			os.Exit(1)
		}

		ver, err := client.Version()
		if err != nil {
			color.Red(err.Error())
			os.Exit(1)
		}

		_prefix = fmt.Sprintf(_prefix, u.Host)

		screen.Clear()
		screen.MoveTopLeft()
		fmt.Printf(header, version(), ver.PkgVersion, time.Now().String(), "✅ Meilisearch is healthy")
		fmt.Println()
	}

	root.AddCommand(healthCmd())
	root.AddCommand(indexCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(statsCmd())
	root.AddCommand(dumpCmd())

	if err := sh.Execute(); err != nil {
		log.Fatal(err)
	}
}

func livePrefix() (string, bool) {
	return _prefix, true
}

func indexCmd() *cobra.Command {
	idx := &cobra.Command{
		Use:   "index",
		Short: "manage index",
	}

	get := &cobra.Command{
		Use:   "get",
		Short: "get an index",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index get {uid}'")
				return
			}

			resp, err := client.GetIndex(args[0])
			if err != nil {
				color.Red(err.Error())
				return
			}

			fmt.Printf(`Index UID: %s
Primary Key: %s
Created At: %s
Updated At: %s
`, resp.UID, resp.PrimaryKey, resp.CreatedAt, resp.UpdatedAt)
		},
	}

	limit, offset := int64(0), int64(0)

	list := &cobra.Command{
		Use:   "list",
		Short: "list of indexes",
		Run: func(cmd *cobra.Command, args []string) {
			res, err := client.GetIndexes(&meilisearch.IndexesQuery{
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				color.Red(err.Error())
				return
			}

			for i, result := range res.Results {
				fmt.Printf(`No: %d
Index UID: %s
Primary Key: %s
Created At: %s
Updated At: %s
`, i+1, result.UID, result.PrimaryKey, result.CreatedAt, result.UpdatedAt)
				fmt.Println("---------------------------------")
			}
		},
	}

	list.Flags().Int64Var(&limit, "limit", 0, "set limit for list of index")
	list.Flags().Int64Var(&offset, "offset", 0, "set offset for list of index")

	primaryKey := ""

	create := &cobra.Command{
		Use:   "create",
		Short: "create an index",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index create {uid}'")
				return
			}
			resp, err := client.CreateIndex(&meilisearch.IndexConfig{
				Uid:        args[0],
				PrimaryKey: primaryKey,
			})
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTask(resp)
		},
	}

	create.Flags().StringVar(&primaryKey, "primary-key", "", "set primary key")

	del := &cobra.Command{
		Use:   "delete",
		Short: "delete an index",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index create {uid}'")
				return
			}
			resp, err := client.DeleteIndex(args[0])
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTask(resp)
		},
	}

	idx.AddCommand(get)
	idx.AddCommand(list)
	idx.AddCommand(create)
	idx.AddCommand(del)

	return idx
}

func multiSearchCmd() *cobra.Command {
	ms := &cobra.Command{
		Use:   "multi-search",
		Short: "do multi search",
		Run: func(cmd *cobra.Command, args []string) {
			client.MultiSearch(&meilisearch.MultiSearchRequest{})
		},
	}

	return ms
}

func dumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "create meilisearch dump",
		Long:  "https://www.meilisearch.com/docs/reference/api/dump",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.CreateDump()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTask(resp)
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "stats of Meilisearch",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.GetStats()
			if err != nil {
				color.Red(err.Error())
				return
			}

			indexes := make([]string, 0, len(resp.Indexes))
			for key, _ := range resp.Indexes {
				indexes = append(indexes, key)
			}

			fmt.Printf(`Database Size: %s
Last Update: %s
Indexes: %s
`, util.FormatBytesToHumanReadable(uint64(resp.DatabaseSize)), resp.LastUpdate, strings.Join(indexes, ", "))
		},
	}
}

func healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "check Meilisearch is healthy",
		Run: func(cmd *cobra.Command, args []string) {
			if ok := client.IsHealthy(); !ok {
				color.Red("❌ Meilisearch is unhealthy")
				return
			}

			color.Green("✅ Meilisearch is healthy")
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Meilisearch",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.Version()
			if err != nil {
				color.Red(err.Error())
				return
			}

			fmt.Printf(`Version: %s
Commit SHA: %s
Commit Date: %s
`, resp.PkgVersion, resp.CommitSha, resp.CommitDate)
		},
	}
}

func printTask(t *meilisearch.TaskInfo) {
	fmt.Printf(`Task UID: %d
Index UID: %s
Status: %s
Type: %s
Enqueued At: %s
`, t.TaskUID, t.IndexUID, t.Status, t.Type, t.EnqueuedAt)

}
