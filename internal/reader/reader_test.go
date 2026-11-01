package reader

import (
	"net/url"
	"testing"
)

func TestToMarkdown(t *testing.T) {
	base, err := url.Parse("https://example.com/feed/")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		src  string
		base *url.URL
		want string
	}{
		{
			name: "headings",
			src:  `<h1>Title</h1><h2>Sub <em>title</em></h2>`,
			want: "# Title\n\n## Sub *title*",
		},
		{
			name: "paragraphs and hard breaks",
			src:  `<p>Hello <b>world</b>.</p><p>Next<br>line</p>`,
			want: "Hello **world**.\n\nNext  \nline",
		},
		{
			name: "relative and unsafe links",
			src:  `<p>Read <a href="/post/1">the post</a> or <a href="javascript:alert(1)">click</a> or <a href="data:text/html,x">data</a>.</p>`,
			base: base,
			want: "Read [the post](https://example.com/post/1) or click or data.",
		},
		{
			name: "protocol relative url and destination escaping",
			src:  `<p><a href="//cdn.example.com/a b(c)">x</a></p>`,
			base: base,
			want: "[x](https://cdn.example.com/a%20b%28c%29)",
		},
		{
			name: "relative image and tracking pixel",
			src:  `<p><img src="img/a.png" alt="A"> <img src="pixel.gif" width="1" height="1" alt="px"></p>`,
			base: base,
			want: "![A](https://example.com/feed/img/a.png)",
		},
		{
			name: "nested unordered list",
			src:  `<ul><li>one<ul><li>two</li></ul></li><li>three</li></ul>`,
			want: "- one\n  - two\n- three",
		},
		{
			name: "nested ordered list",
			src:  `<ol><li>a</li><li>b<ol><li>c</li></ol></li></ol>`,
			want: "1. a\n2. b\n  1. c",
		},
		{
			name: "nested blockquote",
			src:  `<blockquote><p>Outer</p><blockquote><p>Inner</p></blockquote></blockquote>`,
			want: "> Outer\n>\n> > Inner",
		},
		{
			name: "fenced code with language and backticks",
			src:  "<pre><code class=\"language-go\">fmt.Println(\"```\")\nline2</code></pre>",
			want: "````go\nfmt.Println(\"```\")\nline2\n````",
		},
		{
			name: "gfm table with pipes and formatting",
			src:  `<table><tr><th>Name</th><th>Value</th></tr><tr><td>a|b</td><td><strong>2</strong></td></tr><tr><td>only</td></tr></table>`,
			want: "| Name | Value |\n| --- | --- |\n| a\\|b | 2 |\n| only |  |",
		},
		{
			name: "scripts styles and hidden nodes removed",
			src:  `<style>p{}</style><script>alert(1)</script><div style="display:none">hidden</div><p aria-hidden="true">x</p><p>shown</p>`,
			want: "shown",
		},
		{
			name: "plain text escapes line starts",
			src:  "# not heading\n- not list\n1. not ordered\n> not quote\n* not bullet\n+ not bullet\n\nnormal text",
			want: "\\# not heading\n\\- not list\n1\\. not ordered\n\\> not quote\n\\* not bullet\n\\+ not bullet\n\nnormal text",
		},
		{
			name: "nil base keeps only absolute urls",
			src:  `<p><a href="/rel">rel</a> <a href="https://x.test/a">abs</a> <img src="/i.png" alt="i"></p>`,
			want: "rel [abs](https://x.test/a)",
		},
		{
			name: "cjk spacing and emphasis",
			src:  `<p>中文 文本，hello <b>世界</b></p>`,
			want: "中文文本，hello **世界**",
		},
		{
			name: "figure image and caption",
			src:  `<figure><img src="/i.png" alt="Pic"><figcaption>A caption</figcaption></figure>`,
			base: base,
			want: "![Pic](https://example.com/i.png)\n\n*A caption*",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToMarkdown(tt.src, tt.base); got != tt.want {
				t.Fatalf("ToMarkdown() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestPlainText(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "entities and whitespace",
			src:  "<p>Hello&nbsp;&amp;&nbsp; world</p>\n<p>second\n line</p>",
			want: "Hello & world\n\nsecond line",
		},
		{
			name: "drops markup breaks and hidden nodes",
			src:  `<h1>Title</h1><p>One <strong>two</strong><br>three</p><script>x</script><div style="visibility:hidden">no</div>`,
			want: "Title\n\nOne two\nthree",
		},
		{
			name: "nested list text",
			src:  `<ul><li>one</li><li>two<ul><li>three</li></ul></li></ul>`,
			want: "one\ntwo\n  three",
		},
		{
			name: "plain input",
			src:  "one\n two\n\nthree",
			want: "one two\n\nthree",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlainText(tt.src); got != tt.want {
				t.Fatalf("PlainText() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		name string
		src  string
		n    int
		want string
	}{
		{
			name: "chinese rune boundary",
			src:  "<p>这是一个测试文本用于摘要</p>",
			n:    4,
			want: "这是一个",
		},
		{
			name: "does not split a word",
			src:  "<p>hello world again</p>",
			n:    8,
			want: "hello",
		},
		{
			name: "short text unchanged",
			src:  "<p>hi there</p>",
			n:    20,
			want: "hi there",
		},
		{
			name: "no trailing space",
			src:  "<p>hello world</p>",
			n:    5,
			want: "hello",
		},
		{
			name: "zero length",
			src:  "<p>hello</p>",
			n:    0,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summary(tt.src, tt.n); got != tt.want {
				t.Fatalf("Summary(%q, %d) = %q, want %q", tt.src, tt.n, got, tt.want)
			}
		})
	}
}
