package feed

import "net/url"

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
	Items   []rssItem  `xml:"item"`
	Images  []rssImage `xml:"image"`
}

type rdfDocument struct {
	Channel rssChannel `xml:"channel"`
	Items   []rssItem  `xml:"item"`
	Images  []rssImage `xml:"image"`
}

func (r *rdfDocument) document() *rssDocument {
	return &rssDocument{Channel: r.Channel, Items: r.Items, Images: r.Images}
}

type rssChannel struct {
	Title          string     `xml:"title"`
	Link           string     `xml:"link"`
	Description    string     `xml:"description"`
	Language       string     `xml:"language"`
	ManagingEditor string     `xml:"managingEditor"`
	Author         string     `xml:"author"`
	Creator        string     `xml:"creator"`
	Images         []rssImage `xml:"image"`
	Items          []rssItem  `xml:"item"`
}

type rssImage struct {
	URL      string `xml:"url"`
	Href     string `xml:"href,attr"`
	Resource string `xml:"resource,attr"`
}

type rssGUID struct {
	Value string `xml:",chardata"`
}

type rssResource struct {
	URL    string `xml:"url,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr"`
	Medium string `xml:"medium,attr"`
}

type rssItem struct {
	Title         string        `xml:"title"`
	Link          string        `xml:"link"`
	Description   string        `xml:"description"`
	Encoded       string        `xml:"encoded"`
	PubDate       string        `xml:"pubDate"`
	Author        string        `xml:"author"`
	Creator       string        `xml:"creator"`
	GUID          rssGUID       `xml:"guid"`
	About         string        `xml:"about,attr"`
	Date          string        `xml:"date"`
	Enclosures    []rssResource `xml:"enclosure"`
	MediaContents []rssResource `xml:"content"`
	Thumbnails    []rssResource `xml:"thumbnail"`
}

func fromRSS(raw *rssDocument, base *url.URL, typ string) *Document {
	channel := &raw.Channel
	doc := &Document{
		Feed: Feed{
			Title:       PlainText(channel.Title),
			Link:        resolveURL(base, channel.Link),
			Description: feedText(channel.Description),
			Language:    trimmed(channel.Language),
			IconURL:     resolveURL(base, rssIcon(channel.Images, raw.Images)),
			Author:      firstNonEmpty(channel.ManagingEditor, channel.Author, channel.Creator),
			Type:        typ,
		},
	}
	items := make([]rssItem, 0, len(channel.Items)+len(raw.Items))
	items = append(items, channel.Items...)
	items = append(items, raw.Items...)
	doc.Items = make([]Item, 0, len(items))
	for i := range items {
		doc.Items = append(doc.Items, rssItemToItem(&items[i], base))
	}
	return doc
}

func rssIcon(channelImages, documentImages []rssImage) string {
	var href, resource string
	for _, images := range [][]rssImage{channelImages, documentImages} {
		for _, img := range images {
			if u := trimmed(img.URL); u != "" {
				return u
			}
			if href == "" {
				href = trimmed(img.Href)
			}
			if resource == "" {
				resource = trimmed(img.Resource)
			}
		}
	}
	return firstNonEmpty(href, resource)
}

func rssItemToItem(raw *rssItem, base *url.URL) Item {
	link := resolveURL(base, raw.Link)
	if trimmed(raw.Link) == "" {
		// RSS 1.0 items often only carry rdf:about.
		link = resolveURL(base, raw.About)
	}

	description := trimmed(raw.Description)
	content := trimmed(raw.Encoded)
	if content == "" {
		content = description
	}

	author := trimmed(raw.Creator)
	if author == "" {
		author = trimmed(raw.Author)
	}

	var image, enclosure string
	for _, thumb := range raw.Thumbnails {
		if u := resolveURL(base, thumb.URL); u != "" {
			image = u
			break
		}
	}
	for _, enc := range raw.Enclosures {
		if u := resolveURL(base, enc.URL); u != "" {
			enclosure = u
			break
		}
	}
	for _, media := range raw.MediaContents {
		u := resolveURL(base, media.URL)
		if u == "" {
			continue
		}
		if enclosure == "" {
			enclosure = u
		}
		if image == "" && isImageResource(media.Type, media.Medium, media.URL) {
			image = u
		}
	}

	guid := trimmed(raw.GUID.Value)
	if guid == "" {
		guid = trimmed(raw.About)
	}
	if guid == "" {
		guid = link
	}

	published, _ := ParseDate(raw.PubDate)
	if published.IsZero() {
		published, _ = ParseDate(raw.Date)
	}

	return Item{
		GUID:         guid,
		Link:         link,
		Title:        PlainText(raw.Title),
		Author:       author,
		Summary:      PlainText(description),
		Content:      content,
		ImageURL:     image,
		EnclosureURL: enclosure,
		Published:    published,
	}
}
