package main

import (
	"errors"
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
	"sort"
	"strconv"
	"strings"
	"time"
)

const header = `Welcome to MeiliShell %s | https://github.com/Ja7ad/meilishell

- Server: Meilisearch v%s
- Status: %s
- Time: %s
- Docs: https://www.meilisearch.com/docs/reference/api/overview
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
	_prefix = ""
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
		connect(*h, *k, cleanSc, true)
	}

	idxCmd := indexCmd()

	root.AddCommand(healthCmd())
	root.AddCommand(clsCmd())
	root.AddCommand(idxCmd)
	root.AddCommand(KeyCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(statsCmd())
	root.AddCommand(dumpCmd())
	root.AddCommand(connectCmd())
	root.AddCommand(taskCmd())
	root.AddCommand(indexSettingsCmd(idxCmd))

	// TODO currently not support search in shell
	root.AddCommand(multiSearchCmd())
	root.AddCommand(searchCmd())
	root.AddCommand(facetSearch())

	if err := sh.Execute(); err != nil {
		log.Fatal(err)
	}
}

func livePrefix() (string, bool) {
	return _prefix, true
}

func clsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear screen",
		Run: func(cmd *cobra.Command, args []string) {
			cleanSc()
		},
	}
}

func connectCmd() *cobra.Command {
	key := ""

	c := &cobra.Command{
		Use:   "connect",
		Short: "connect to another Meilisearch",
		Long:  "connect http://localhost:7700 --api-key foobar",
	}

	c.Flags().StringVar(&key, "api-key", "", "set meilisearch api key or master key "+
		"(https://www.meilisearch.com/docs/reference/api/keys)")

	c.Run = func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			color.Red("host is require as argument, for example 'connect http://localhost:7700 --api-key foobar'")
			return
		}

		connect(args[0], key, nil, false)
	}

	return c
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

			sort.Slice(res.Results, func(i, j int) bool {
				return i > j
			})

			for i, result := range res.Results {
				fmt.Printf(`No: %d
Index UID: %s
Primary Key: %s
Created At: %s
Updated At: %s
`, i+1, result.UID, result.PrimaryKey, result.CreatedAt, result.UpdatedAt)
				lineBreaker()
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

			printTaskInfo(resp)
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

			printTaskInfo(resp)
		},
	}

	swap := &cobra.Command{
		Use:   "swap",
		Short: "swap indexes",
		Long:  "swap foo,bar a,b x,y",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'swap foo,bar x,y a,b'")
				return
			}

			swaps := make([]meilisearch.SwapIndexesParams, 0)

			for _, arg := range args {
				indexes := strings.Split(arg, ",")
				if len(indexes) == 2 {
					swaps = append(swaps, meilisearch.SwapIndexesParams{
						Indexes: indexes,
					})
				}
			}

			res, err := client.SwapIndexes(swaps)
			if err != nil {
				color.Red(err.Error())
			}

			printTaskInfo(res)
		},
	}

	idx.AddCommand(get)
	idx.AddCommand(list)
	idx.AddCommand(create)
	idx.AddCommand(del)
	idx.AddCommand(swap)

	return idx
}

func indexSettingsCmd(idxCmd *cobra.Command) *cobra.Command {
	settings := &cobra.Command{
		Use:   "settings",
		Short: "manage settings",
	}

	get := &cobra.Command{
		Use:   "get",
		Short: "get settings",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetSettings()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printSettings(res)
		},
	}

	getRankingRules := &cobra.Command{
		Use:   "ranking-rules",
		Short: "get ranking rules",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get ranking-rules {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetRankingRules()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Ranking Rules: %v\n", strings.Join(*res, ","))
			}
		},
	}

	getDistinctAttribute := &cobra.Command{
		Use:   "distinct-attribute",
		Short: "get distinct attribute",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get distinct-attribute {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetDistinctAttribute()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Distinct Attribute: %s\n", *res)
			}
		},
	}

	getSearchableAttributes := &cobra.Command{
		Use:   "searchable-attributes",
		Short: "get searchable attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get searchable-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetSearchableAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Searchable Attributes: %s\n", strings.Join(*res, ","))
			}
		},
	}

	getDisplayedAttributes := &cobra.Command{
		Use:   "displayed-attributes",
		Short: "get displayed attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get displayed-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetDisplayedAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Displayed Attributes: %s\n", strings.Join(*res, ","))
			}
		},
	}

	getStopWords := &cobra.Command{
		Use:   "stop-words",
		Short: "get stop words",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get stop-words {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetStopWords()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Stop Words: %s\n", strings.Join(*res, ","))
			}
		},
	}

	getSynonyms := &cobra.Command{
		Use:   "synonyms",
		Short: "get synonyms",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get synonyms {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetSynonyms()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Synonyms: %+v\n", *res)
			}
		},
	}

	getFilterableAttributes := &cobra.Command{
		Use:   "filterable-attributes",
		Short: "get filterable attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get filterable-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetFilterableAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Filterable Attributes: %s\n", strings.Join(*res, ","))
			}
		},
	}

	getSortableAttributes := &cobra.Command{
		Use:   "sortable-attributes",
		Short: "get sortable attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get sortable-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetSortableAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Sortable Attributes: %s\n", strings.Join(*res, ","))
			}
		},
	}

	getTypoTolerance := &cobra.Command{
		Use:   "typo-tolerance",
		Short: "get typo-tolerance",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get typo-tolerance {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetTypoTolerance()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Typo Tolerance: %+v\n", *res)
			}
		},
	}

	getPagination := &cobra.Command{
		Use:   "pagination",
		Short: "get pagination",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get pagination {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetPagination()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Pagination: %+v\n", *res)
			}
		},
	}

	getFaceting := &cobra.Command{
		Use:   "faceting",
		Short: "get faceting",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get faceting {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetFaceting()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Faceting: %+v\n", *res)
			}
		},
	}

	getEmbedders := &cobra.Command{
		Use:   "embedders",
		Short: "get embedders",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get embedders {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetEmbedders()
			if err != nil {
				color.Red(err.Error())
				return
			}

			if res != nil {
				fmt.Printf("Embedders: %+v\n", res)
			}
		},
	}

	getSearchCutoffMs := &cobra.Command{
		Use:   "search-cutoff-ms",
		Short: "get search cutoff ms",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings get search-cutoff-ms {uid}'")
				return
			}

			res, err := client.Index(args[0]).GetSearchCutoffMs()
			if err != nil {
				color.Red(err.Error())
				return
			}

			fmt.Printf("Search cutoff ms: %d\n", res)
		},
	}

	get.AddCommand(getRankingRules)
	get.AddCommand(getDistinctAttribute)
	get.AddCommand(getSearchableAttributes)
	get.AddCommand(getDisplayedAttributes)
	get.AddCommand(getStopWords)
	get.AddCommand(getSynonyms)
	get.AddCommand(getFilterableAttributes)
	get.AddCommand(getSortableAttributes)
	get.AddCommand(getTypoTolerance)
	get.AddCommand(getPagination)
	get.AddCommand(getFaceting)
	get.AddCommand(getEmbedders)
	get.AddCommand(getSearchCutoffMs)

	update := &cobra.Command{
		Use:   "update",
		Short: "update settings",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings update {uid}'")
				return
			}

			color.Cyan("currently not support update settings")
		},
	}

	reset := &cobra.Command{
		Use:   "reset",
		Short: "reset settings",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetSettings()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetRankingRules := &cobra.Command{
		Use:   "ranking-rules",
		Short: "reset ranking rules",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset ranking-rules {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetRankingRules()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetDistinctAttribute := &cobra.Command{
		Use:   "distinct-attribute",
		Short: "reset distinct attribute",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset distinct-attribute {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetDistinctAttribute()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetSearchableAttributes := &cobra.Command{
		Use:   "searchable-attributes",
		Short: "reset searchable attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset searchable-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetSearchableAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetDisplayedAttributes := &cobra.Command{
		Use:   "displayed-attributes",
		Short: "reset displayed attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset displayed-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetDisplayedAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetStopWords := &cobra.Command{
		Use:   "stop-words",
		Short: "reset stop words",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset stop-words {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetStopWords()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetSynonyms := &cobra.Command{
		Use:   "synonyms",
		Short: "reset synonyms",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset synonyms {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetSynonyms()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetFilterableAttributes := &cobra.Command{
		Use:   "filterable-attributes",
		Short: "reset filterable attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset filterable-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetFilterableAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetSortableAttributes := &cobra.Command{
		Use:   "sortable-attributes",
		Short: "reset sortable attributes",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset sortable-attributes {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetSortableAttributes()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetTypoTolerance := &cobra.Command{
		Use:   "typo-tolerance",
		Short: "reset typo-tolerance",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset typo-tolerance {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetTypoTolerance()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetPagination := &cobra.Command{
		Use:   "pagination",
		Short: "reset pagination",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset pagination {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetPagination()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetFaceting := &cobra.Command{
		Use:   "faceting",
		Short: "reset faceting",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset faceting {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetFaceting()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetEmbedders := &cobra.Command{
		Use:   "embedders",
		Short: "reset embedders",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset embedders {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetEmbedders()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	resetSearchCutoffMs := &cobra.Command{
		Use:   "search-cutoff-ms",
		Short: "reset search cutoff ms",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("index uid is require 'index settings reset search-cutoff-ms {uid}'")
				return
			}

			res, err := client.Index(args[0]).ResetSearchCutoffMs()
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	reset.AddCommand(resetRankingRules)
	reset.AddCommand(resetDistinctAttribute)
	reset.AddCommand(resetSearchableAttributes)
	reset.AddCommand(resetDisplayedAttributes)
	reset.AddCommand(resetStopWords)
	reset.AddCommand(resetSynonyms)
	reset.AddCommand(resetFilterableAttributes)
	reset.AddCommand(resetSortableAttributes)
	reset.AddCommand(resetTypoTolerance)
	reset.AddCommand(resetPagination)
	reset.AddCommand(resetFaceting)
	reset.AddCommand(resetEmbedders)
	reset.AddCommand(resetSearchCutoffMs)

	settings.AddCommand(get)
	settings.AddCommand(update)
	settings.AddCommand(reset)

	idxCmd.AddCommand(settings)
	return settings
}

func multiSearchCmd() *cobra.Command {
	ms := &cobra.Command{
		Use:   "multi-search",
		Short: "do multi search",
		Run: func(cmd *cobra.Command, args []string) {
			color.Cyan("TODO: how to get multiple search queries?")
		},
	}

	return ms
}

func searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search",
		Short: "search index",
		Long:  "https://www.meilisearch.com/docs/reference/api/search",
		Run: func(cmd *cobra.Command, args []string) {
			color.Cyan("currently not support search index in shell")
		},
	}
}

func facetSearch() *cobra.Command {
	return &cobra.Command{
		Use:   "facet-search",
		Short: "facet search index",
		Long:  "https://www.meilisearch.com/docs/reference/api/facet_search",
		Run: func(cmd *cobra.Command, args []string) {
			color.Cyan("currently not support search index in shell")
		},
	}
}

func KeyCmd() *cobra.Command {
	key := &cobra.Command{
		Use:   "key",
		Short: "manage keys",
		Long:  "https://www.meilisearch.com/docs/reference/api/keys",
	}

	actions := make([]string, 0)
	indexes := make([]string, 0)
	expireAt := ""
	uid := ""
	name := ""
	description := ""

	create := &cobra.Command{
		Use:   "create",
		Short: "create one key",
		Run: func(cmd *cobra.Command, args []string) {
			if len(actions) == 0 {
				color.Red("key actions is require, please see --help")
				return
			}

			if len(indexes) == 0 {
				color.Red("key indexes is require, please see --help")
				return
			}

			if len(expireAt) == 0 {
				color.Red("key expireAt is require, please see --help")
				return
			}

			t, err := time.Parse("2006-01-02T15:04:05Z", expireAt)
			if err != nil {
				color.Red(err.Error())
				return
			}

			res, err := client.CreateKey(&meilisearch.Key{
				Name:        name,
				Description: description,
				UID:         uid,
				Actions:     actions,
				Indexes:     indexes,
				ExpiresAt:   t,
			})
			if err != nil {
				color.Red(err.Error())
				return
			}

			printKey(res)
		},
	}

	create.Flags().StringSliceVar(&actions, "actions", nil,
		"A list of API actions permitted for the key. [*] for all actions")

	create.Flags().StringSliceVar(&indexes, "indexes", nil,
		"An array of indexes the key is authorized to act on. [*] for all indexes")

	create.Flags().StringVar(&expireAt, "expire-at", "",
		"Date and time when the key will expire, represented in RFC 3339 format")

	create.Flags().StringVar(&uid, "uid", "",
		"A uuid v4 to identify the API key. If not specified, it is generated by Meilisearch")

	create.Flags().StringVar(&name, "name", "", "set name for the key")

	create.Flags().StringVar(&description, "description", "", "set description for the key")

	limit, offset := int64(0), int64(0)

	list := &cobra.Command{
		Use:   "list",
		Short: "list all keys",
		Run: func(cmd *cobra.Command, args []string) {
			res, err := client.GetKeys(&meilisearch.KeysQuery{
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				color.Red(err.Error())
				return
			}

			sort.Slice(res.Results, func(i, j int) bool {
				return i > j
			})

			for _, result := range res.Results {
				printKey(&result)
				lineBreaker()
			}
		},
	}

	list.Flags().Int64Var(&limit, "limit", 0, "set limit for list of key")
	list.Flags().Int64Var(&offset, "offset", 0, "set offset for list of key")

	get := &cobra.Command{
		Use:   "get",
		Short: "get one key",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("key uid is require identifier (key or uid) 'key get {identifier}'")
				return
			}

			res, err := client.GetKey(args[0])
			if err != nil {
				color.Red(err.Error())
				return
			}

			printKey(res)
		},
	}

	update := &cobra.Command{
		Use:   "update",
		Short: "update an key",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("key uid is require identifier (key or uid) 'key update {identifier}'")
				return
			}

			identifier := args[0]

			res, err := client.UpdateKey(identifier, &meilisearch.Key{
				Name:        name,
				Description: description,
			})

			if err != nil {
				color.Red(err.Error())
				return
			}

			printKey(res)
		},
	}

	update.Flags().StringVar(&name, "name", "", "set name for the key")

	update.Flags().StringVar(&description, "description", "", "set description for the key")

	del := &cobra.Command{
		Use:   "delete",
		Short: "delete an key",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("key uid is require identifier (key or uid) 'key delete {identifier}'")
				return
			}

			identifier := args[0]

			res, err := client.DeleteKey(identifier)
			if err != nil {
				color.Red(err.Error())
				return
			}

			fmt.Println(res)
		},
	}

	key.AddCommand(create)
	key.AddCommand(list)
	key.AddCommand(get)
	key.AddCommand(update)
	key.AddCommand(del)

	return key
}

func taskCmd() *cobra.Command {
	task := &cobra.Command{
		Use:   "task",
		Short: "manage tasks",
	}

	get := &cobra.Command{
		Use:   "get",
		Short: "get a task",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("task uid is require 'task get {task_uid}'")
				return
			}

			uid, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				color.Red(err.Error())
				return
			}

			t, err := client.GetTask(uid)
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTask(t)
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "list all tasks",
		Run: func(cmd *cobra.Command, args []string) {
			// TODO we can support task list params?
			res, err := client.GetTasks(nil)
			if err != nil {
				color.Red(err.Error())
				return
			}

			sort.Slice(res.Results, func(i, j int) bool {
				return i > j
			})

			for _, t := range res.Results {
				printTask(&t)
				lineBreaker()
			}
		},
	}

	cancel := &cobra.Command{
		Use:   "cancel",
		Short: "cancel a or many tasks",
		Long:  "task cancel 1 2 3 4",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("task uid is require 'task cancel {task_uid or many 1 2 3 4}'")
				return
			}

			uids := make([]int64, 0)

			for _, arg := range args {
				uid, err := strconv.ParseInt(arg, 10, 64)
				if err != nil {
					color.Red(err.Error())
					return
				}
				uids = append(uids, uid)
			}

			res, err := client.CancelTasks(&meilisearch.CancelTasksQuery{
				UIDS: uids,
			})

			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	del := &cobra.Command{
		Use:   "delete",
		Short: "delete a or many tasks",
		Long:  "task delete 1 2 3 4",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				color.Red("task uid is require 'task cancel {task_uid or many 1 2 3 4}'")
				return
			}

			uids := make([]int64, 0)

			for _, arg := range args {
				uid, err := strconv.ParseInt(arg, 10, 64)
				if err != nil {
					color.Red(err.Error())
					return
				}
				uids = append(uids, uid)
			}

			res, err := client.DeleteTasks(&meilisearch.DeleteTasksQuery{
				UIDS: uids,
			})
			if err != nil {
				color.Red(err.Error())
				return
			}

			printTaskInfo(res)
		},
	}

	task.AddCommand(get)
	task.AddCommand(list)
	task.AddCommand(cancel)
	task.AddCommand(del)

	return task
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

			printTaskInfo(resp)
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

func printTaskInfo(t *meilisearch.TaskInfo) {
	fmt.Printf(`Task UID: %d
Index UID: %s
Status: %s
Type: %s
Enqueued At: %s
`, t.TaskUID, t.IndexUID, t.Status, t.Type, t.EnqueuedAt)

}

func printTask(t *meilisearch.Task) {
	fmt.Printf(`Task UID: %d
Index UID: %s
UID: %d
Status: %s
Type: %s
Error: %s
Duration: %s
Enqueued At: %s
Started At: %s
Finished At: %s
Details: %+v
CanceledBy: %d
`, t.TaskUID, t.IndexUID, t.UID, t.Status,
		t.Type, t.Error.Message, t.Duration, t.EnqueuedAt, t.StartedAt, t.FinishedAt, t.Details, t.CanceledBy,
	)
}

func printKey(k *meilisearch.Key) {
	expire := k.ExpiresAt.String()
	if k.ExpiresAt.IsZero() {
		expire = "no expire"
	}

	fmt.Printf(`Name: %s
Description: %s
Key: %s
UID: %s
Actions: %s
Indexes: %s
ExpiresAt: %s
CreatedAt: %s
UpdatedAt: %s
`, k.Name, k.Description, k.Key, k.UID, strings.Join(k.Actions, ", "), strings.Join(k.Indexes, ", "),
		expire, k.CreatedAt, k.UpdatedAt)
}

func printSettings(set *meilisearch.Settings) {
	distinctAttribute := ""
	typoTolerance := meilisearch.TypoTolerance{}
	pagination := meilisearch.Pagination{}
	faceting := meilisearch.Faceting{}

	if set.DistinctAttribute != nil {
		distinctAttribute = *set.DistinctAttribute
	}

	if set.TypoTolerance != nil {
		typoTolerance = *set.TypoTolerance
	}

	if set.Pagination != nil {
		pagination = *set.Pagination
	}

	if set.Faceting != nil {
		faceting = *set.Faceting
	}

	fmt.Printf(`Ranking Rules: %+v
DistinctAttribute: %s
SearchableAttributes: %+v
SearchCutoffMs: %d
DisplayedAttributes: %+v
StopWords: %+v
Synonyms: %v
FilterableAttributes: %+v
SortableAttributes: %+v
TypoTolerance: %+v
Pagination: %+v
Faceting: %+v
Embedders: %+v
`, strings.Join(set.RankingRules, ","),
		distinctAttribute,
		strings.Join(set.SearchableAttributes, ","),
		set.SearchCutoffMs,
		strings.Join(set.DisplayedAttributes, ","),
		strings.Join(set.StopWords, ","),
		set.Synonyms,
		strings.Join(set.FilterableAttributes, ","),
		strings.Join(set.SortableAttributes, ","),
		typoTolerance,
		pagination,
		faceting,
		set.Embedders,
	)
}

func lineBreaker() {
	fmt.Println("---------------------------------")
}

func connect(host, key string, cleanFunc func(), exit bool) {
	u, err := url.Parse(host)
	if err != nil {
		color.Red(err.Error())
		if exit {
			os.Exit(1)
		}
		return
	}

	client = meilisearch.NewClient(meilisearch.ClientConfig{
		Host:   u.String(),
		APIKey: key,
	})

	if !client.IsHealthy() {
		color.Red("❌ Failed connect to Meilisearch, Host or API-Key is invalid")
		if exit {
			os.Exit(1)
		}
		return
	}

	ver, err := client.Version()
	if err != nil {
		e := new(meilisearch.Error)
		ok := errors.As(err, &e)
		if ok {
			if e.StatusCode == 401 || e.StatusCode == 403 {
				color.Red("master key is invalid, 'meilishell --api-key foobar'")
				if exit {
					os.Exit(1)
				}
				return
			}
		}
		color.Red(err.Error())
		if exit {
			os.Exit(1)
		}
		return
	}

	_prefix = fmt.Sprintf("Meilishell@%s > ", u.Host)

	if cleanFunc != nil {
		cleanFunc()
	}

	fmt.Printf(header, version(), ver.PkgVersion, "✅ Meilisearch is healthy", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()
}

func cleanSc() {
	screen.Clear()
	screen.MoveTopLeft()
}
