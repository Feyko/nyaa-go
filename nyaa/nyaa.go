package nyaa

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/docker/go-units"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var NyaaURL = "https://nyaa.si"

func Search(search string, parameters ...SearchParameters) ([]Media, error) {
	params, err := getOneParameterSet(parameters)
	if err != nil {
		return nil, err
	}

	doc, err := requestHTML(search, params)
	if err != nil {
		return nil, errors.Wrap(err, "error getting the nyaa page")
	}

	medias, err := parseSearchPageHTML(doc)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing html")
	}

	return medias, nil
}

func getOneParameterSet(parameters []SearchParameters) (SearchParameters, error) {
	params := SearchParameters{}
	if len(parameters) == 1 {
		params = parameters[0]
	}
	if len(parameters) > 1 {
		return SearchParameters{}, errors.New("only one parameter set accepted")
	}

	return params, nil
}

func requestHTML(search string, params SearchParameters) (*goquery.Document, error) {
	URL, err := urlForParams(search, params)
	if err != nil {
		return nil, errors.Wrap(err, "error creating url for search")
	}

	rep, err := http.Get(URL)
	if err != nil {
		return nil, errors.Wrap(err, "error requesting results")
	}
	defer rep.Body.Close()

	if rep.StatusCode < 200 || rep.StatusCode >= 300 {
		return nil, errors.Errorf("non-OK HTTP status code: %d %s", rep.StatusCode, rep.Status)
	}

	doc, err := goquery.NewDocumentFromReader(rep.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing response html")
	}

	return doc, nil
}

func urlForParams(search string, parameters SearchParameters) (string, error) {
	baseURL := NyaaURL
	if parameters.User != "" {
		baseURL += "/user/" + url.PathEscape(parameters.User)
	}

	URL, err := url.Parse(NyaaURL)
	if err != nil {
		return "", errors.Wrap(err, "error parsing nyaa url")
	}

	query := URL.Query()
	query.Set("f", strconv.FormatInt(int64(parameters.Filter), 10))
	query.Set("c", string(parameters.Category))
	query.Set("q", search)
	query.Set("s", string(parameters.SortBy))
	query.Set("o", string(parameters.SortOrder))
	URL.RawQuery = query.Encode()

	return URL.String(), nil
}

func parseSearchPageHTML(doc *goquery.Document) ([]Media, error) {
	selection := doc.Find(".torrent-list tbody tr")
	medias := make([]Media, selection.Length())
	errChan := make(chan error)
	group := sync.WaitGroup{}
	group.Add(selection.Length())
	doneChan := make(chan struct{})
	go func() {
		group.Wait()
		doneChan <- struct{}{}
	}()

	go func() {
		selection.Each(func(i int, selection *goquery.Selection) {
			media, err := parseMediaElement(selection)
			if err != nil {
				errChan <- errors.Wrap(err, "error parsing media element")
				return
			}
			medias[i] = media
			group.Done()
		})
	}()

	select {
	case err := <-errChan:
		return nil, err
	case <-doneChan:
	}

	return medias, nil
}

func parseMediaElement(elem *goquery.Selection) (Media, error) {
	media := Media{}

	links := elem.Find("td a:not(.comments)").Nodes
	err := parseMediaElementLinks(links, &media)
	if err != nil {
		return media, err
	}

	nodes := elem.Find("td").Nodes
	err = parseMediaElementTexts(nodes, &media)
	if err != nil {
		return media, err
	}

	err = parseMediaElementTimestamp(nodes, &media)
	if err != nil {
		return media, err
	}

	err = parseMediaElementCommentCount(elem, &media)
	if err != nil {
		return media, err
	}

	return media, nil
}

func parseMediaElementLinks(links []*html.Node, media *Media) error {
	if len(links) != 4 {
		return errors.Errorf("unexpected layout: expected 4 links, got %d", len(links))
	}

	href, ok := getAttributeValueByKey(links[0], "href")
	if !ok {
		return errors.New("unexpected layout: first link does not have an href")
	}
	media.Category = hrefToCategory(href)

	href, ok = getAttributeValueByKey(links[1], "href")
	if !ok {
		return errors.New("unexpected layout: second link does not have an href")
	}
	id, err := hrefToID(href)
	if err != nil {
		return errors.Wrap(err, "error parsing ID")
	}
	media.ID = id
	title, ok := getAttributeValueByKey(links[1], "title")
	if !ok {
		return errors.New("unexpected layout: second link does not have a title")
	}
	media.Name = title

	href, ok = getAttributeValueByKey(links[2], "href")
	if !ok {
		return errors.New("unexpected layout: third link does not have an href")
	}
	media.Torrent = href

	href, ok = getAttributeValueByKey(links[3], "href")
	if !ok {
		return errors.New("unexpected layout: fourth link does not have an href")
	}
	media.Magnet = href

	return nil
}

func parseMediaElementTexts(nodes []*html.Node, media *Media) error {
	if len(nodes) != 8 {
		return errors.Errorf("unexpected layout: expected 8 nodes, got %d", len(nodes))
	}

	if nodes[3].FirstChild == nil || nodes[3].FirstChild.Type != html.TextNode {
		return errors.New("unexpected layout: expected node 4 to have a text first child")
	}
	size, err := units.FromHumanSize(nodes[3].FirstChild.Data)
	if err != nil {
		return errors.Wrap(err, "error parsing size")
	}
	media.Size = uint64(size)

	if nodes[5].FirstChild == nil || nodes[5].FirstChild.Type != html.TextNode {
		return errors.New("unexpected layout: expected node 6 to have a text first child")
	}
	seeders, err := strconv.Atoi(nodes[5].FirstChild.Data)
	if err != nil {
		return errors.Wrap(err, "error parsing se")
	}
	media.Seeders = uint(seeders)

	if nodes[6].FirstChild == nil || nodes[6].FirstChild.Type != html.TextNode {
		return errors.New("unexpected layout: expected node 7 to have a text first child")
	}
	leechers, err := strconv.Atoi(nodes[6].FirstChild.Data)
	if err != nil {
		return errors.Wrap(err, "error parsing leechers")
	}
	media.Leechers = uint(leechers)

	if nodes[7].FirstChild == nil || nodes[7].FirstChild.Type != html.TextNode {
		return errors.New("unexpected layout: expected node 8 to have a text first child")
	}
	downloads, err := strconv.Atoi(nodes[7].FirstChild.Data)
	if err != nil {
		return errors.Wrap(err, "error parsing downloads")
	}
	media.Downloads = uint(downloads)

	return nil
}

func parseMediaElementTimestamp(nodes []*html.Node, media *Media) error {
	timestamp, ok := getAttributeValueByKey(nodes[4], "data-timestamp")
	if !ok {
		return errors.New("unexpected layout: expected node 5 to have a data-timestamp")
	}
	timestampInt, err := strconv.Atoi(timestamp)
	if err != nil {
		return errors.Wrap(err, "error parsing timestamp")
	}
	media.Date = time.Unix(int64(timestampInt), 0)
	return nil
}

func parseMediaElementCommentCount(elem *goquery.Selection, media *Media) error {
	nodes := elem.Find(".comments").Nodes
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) > 1 {
		return errors.New("found more than one comments element")
	}

	textChild := nodes[0].LastChild
	if textChild == nil || textChild.Type != html.TextNode {
		return errors.New("unexpected layout: expected comments elem to have a text last child")
	}
	commentCount, err := strconv.Atoi(textChild.Data)
	if err != nil {
		return errors.Wrap(err, "error parsing comment count")
	}
	media.CommentCount = uint(commentCount)

	return nil
}

func getAttributeValueByKey(node *html.Node, key string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val, true
		}
	}
	return "", false
}

func hrefToCategory(href string) Category {
	return Category(strings.TrimPrefix(href, "/?c="))
}

func hrefToID(href string) (uint, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(href, "/view/"))
	return uint(id), err
}
