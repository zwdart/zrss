package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"xin.dart.zrss/internal/feed"
	"xin.dart.zrss/internal/reader"
	"xin.dart.zrss/internal/storage"
)

const pageSize = 40

// refreshAll fetches every subscription, stores what is new and reloads what
// the window shows. manual asks for a message either way.
func (a *app) refreshAll(manual bool) {
	if a.store == nil || a.quitting.Load() {
		return
	}
	if a.refreshing {
		if manual {
			a.say("正在刷新…")
		}
		return
	}
	if len(a.feeds) == 0 {
		if manual {
			a.say("还没有订阅")
		}
		return
	}
	a.refreshing = true
	a.redraw()
	feeds := append([]storage.Feed(nil), a.feeds...)
	keepArticles := a.settings.KeepArticles
	ctx := context.Background()
	go func() {
		newArticles, failed := 0, 0
		for _, f := range feeds {
			select {
			case <-a.done:
				return
			default:
			}
			n, err := a.refreshFeed(ctx, f)
			newArticles += n
			if err != nil {
				failed++
			}
		}
		if keepArticles > 0 {
			_, _ = a.store.Prune(ctx, keepArticles)
		}
		a.post(func() {
			a.refreshing = false
			a.lastRefresh = time.Now()
			a.saveSettings(ctx)
			a.reloadSidebar(ctx)
			a.reloadArticles(ctx)
			switch {
			case newArticles > 0:
				a.say(fmt.Sprintf("收到 %d 篇新文章", newArticles))
				a.notifyNew(newArticles)
			case failed > 0:
				a.say("部分订阅刷新失败")
			case manual:
				a.say("已是最新")
			}
		})
	}()
}

// refreshOne fetches a single subscription, from its context menu.
func (a *app) refreshOne(ctx context.Context, id int64) {
	f, err := a.store.Feed(ctx, id)
	if err != nil {
		a.say("找不到这个订阅")
		return
	}
	a.say("正在刷新…")
	go func() {
		n, err := a.refreshFeed(ctx, f)
		a.post(func() {
			a.reloadSidebar(ctx)
			a.reloadArticles(ctx)
			switch {
			case err != nil:
				a.say("刷新失败")
			case n > 0:
				a.say(fmt.Sprintf("收到 %d 篇新文章", n))
			default:
				a.say("已是最新")
			}
		})
	}()
}

// refreshFeed fetches one feed and returns how many articles were new.
func (a *app) refreshFeed(ctx context.Context, f storage.Feed) (int, error) {
	etag, lastModified := f.ETag, f.LastModified
	if f.Total == 0 {
		// With no stored articles, a 304 would leave the reader empty
		// forever: the server says "not modified" about a copy this
		// database never managed to keep, for example when the first
		// insert was interrupted. Ask for the whole document instead.
		etag, lastModified = "", ""
	}
	res, err := a.fetcher.Fetch(ctx, f.URL, etag, lastModified)
	now := time.Now()
	if err != nil {
		_ = a.store.UpdateFeed(ctx, f.ID, storage.FeedMeta{
			Title: f.Title, SiteURL: f.SiteURL, Description: f.Description, IconURL: f.IconURL,
			ETag: f.ETag, LastModified: f.LastModified, LastFetch: now, LastError: truncate(err.Error(), 300),
		})
		return 0, err
	}
	if res.NotModified {
		return 0, a.store.UpdateFeed(ctx, f.ID, storage.FeedMeta{
			Title: f.Title, SiteURL: f.SiteURL, Description: f.Description, IconURL: f.IconURL,
			ETag: firstOf(res.ETag, f.ETag), LastModified: firstOf(res.LastModified, f.LastModified),
			LastFetch: now,
		})
	}
	doc := res.Doc
	title := firstOf(doc.Title, f.Title, hostOf(f.URL), f.URL)
	meta := storage.FeedMeta{
		Title:        title,
		SiteURL:      firstOf(doc.Link, f.SiteURL),
		Description:  firstOf(doc.Description, f.Description),
		IconURL:      firstOf(doc.IconURL, f.IconURL),
		ETag:         firstOf(res.ETag, f.ETag),
		LastModified: firstOf(res.LastModified, f.LastModified),
		LastFetch:    now,
	}
	if err := a.store.UpdateFeed(ctx, f.ID, meta); err != nil {
		return 0, err
	}
	n, err := a.store.UpsertArticles(ctx, f.ID, articleInputs(doc, res.FinalURL))
	if err != nil {
		// Forget the validators so the next refresh downloads the feed
		// again instead of answering 304 for articles that are missing.
		meta.ETag, meta.LastModified = "", ""
		_ = a.store.UpdateFeed(ctx, f.ID, meta)
		return 0, err
	}
	return n, nil
}

// articleInputs turns parsed items into what the store keeps: Markdown for
// reading, plain text for searching, and a short summary for the list.
func articleInputs(doc *feed.Document, finalURL string) []storage.ArticleInput {
	base, _ := url.Parse(finalURL)
	out := make([]storage.ArticleInput, 0, len(doc.Items))
	for _, item := range doc.Items {
		linkBase := base
		if item.Link != "" {
			if u, err := url.Parse(item.Link); err == nil {
				linkBase = u
			}
		}
		content := reader.ToMarkdown(item.Content, linkBase)
		if strings.TrimSpace(content) == "" {
			content = reader.ToMarkdown(item.Summary, linkBase)
		}
		plain := reader.PlainText(item.Content)
		if strings.TrimSpace(plain) == "" {
			plain = reader.PlainText(item.Summary)
		}
		summary := reader.Summary(item.Summary, 240)
		if summary == "" {
			summary = reader.Summary(item.Content, 240)
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "无标题"
		}
		published := item.Published
		out = append(out, storage.ArticleInput{
			GUID:      item.GUID,
			URL:       item.Link,
			Title:     title,
			Author:    item.Author,
			Summary:   summary,
			Content:   content,
			Plain:     plain,
			ImageURL:  item.ImageURL,
			Published: published,
		})
	}
	return out
}

// notifyNew shows a desktop notification for new articles when the window is
// in the background.
func (a *app) notifyNew(n int) {
	if !a.settings.NotifyNew || a.win == nil || a.win.IsFocused() || !mygo.NotificationsSupported() {
		return
	}
	note := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "new-articles",
		Title: mygo.App.Name(),
		Body:  fmt.Sprintf("收到 %d 篇新文章", n),
	})
	note.OnClick(a.showMain)
	_ = note.Show()
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
