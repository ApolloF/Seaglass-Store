package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	catalog "github.com/ApolloF/Seaglass/internal/store/sources"
	"io"
	"os"
	"os/signal"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "cataloglab:", err)
		os.Exit(1)
	}
}
func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("cataloglab", flag.ContinueOnError)
	enabled := flags.Bool("private-sources", false, "enable private source adapters for this invocation")
	id := flags.String("source", "", "registered provider ID (see docs/store-providers.md)")
	input := flags.String("input", "", "parse a saved document without network requests")
	page := flags.String("url", "", "source URL or base URL for a saved document")
	search := flags.String("search", "", "find releases through the source's own site search")
	pages := flags.Int("pages", 1, "number of listing/feed pages (1..10)")
	details := flags.Int("follow-details", 5, "enrich up to this many abbreviated results (0..20)")
	resolve := flags.Int("resolve-torrents", 0, "normal file-host attempts for torrent metadata only (0..3)")
	torrentFile := flags.String("torrent-file", "", "attach manually obtained torrent metadata to exactly one selected release")
	query := flags.String("query", "", "filter source titles by these words")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *pages < 1 || *pages > 10 || *details < 0 || *details > 20 || *resolve < 0 || *resolve > 3 {
		return errors.New("pages must be 1..10, details 0..20, torrent resolutions 0..3")
	}
	if *input != "" && (*pages != 1 || *resolve != 0 || *search != "") {
		return errors.New("offline parsing cannot search, paginate or resolve network links")
	}
	if *search != "" && *page != "" {
		return errors.New("choose search or a source URL")
	}
	source, err := catalog.PrivateSource(*id, *enabled)
	if err != nil {
		return err
	}
	if *page == "" {
		*page = source.StartURL
	}
	if *search != "" {
		*page, err = source.SearchURL(*search)
		if err != nil {
			return err
		}
		if *query == "" {
			*query = *search
		}
	}
	if _, err := source.ValidateURL(*page); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var snapshot catalog.Snapshot
	if *input != "" {
		data, err := readBounded(*input, catalog.MaxDocumentBytes)
		if err != nil {
			return err
		}
		entries, err := catalog.Parse(source, *page, data)
		if err != nil {
			return err
		}
		snapshot = catalog.NewSnapshot(source, data, catalog.Search(entries, *query))
	} else {
		client, err := catalog.NewClient(*id, *enabled)
		if err != nil {
			return err
		}
		defer client.Close()
		snapshot, err = catalog.Collect(ctx, client, *page, catalog.CollectOptions{Pages: *pages, Details: *details, Resolve: *resolve, Query: *query})
		if err != nil {
			return err
		}
	}
	if *torrentFile != "" {
		if len(snapshot.Entries) != 1 {
			return errors.New("manual torrent metadata needs exactly one selected release")
		}
		data, err := readBounded(*torrentFile, 2<<20)
		if err != nil {
			return err
		}
		transport, err := catalog.TorrentMetadata(data)
		if err != nil {
			return err
		}
		snapshot.Entries[0].Transports = append(snapshot.Entries[0].Transports, transport)
		snapshot.Entries[0].Warnings = append(snapshot.Entries[0].Warnings, "Manual torrent metadata attached; confirm its game identity before use")
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(snapshot)
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit {
		return nil, errors.New("input file exceeds its size limit")
	}
	return data, nil
}
