package lark

import (
	"strconv"
	"strings"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

// postElement represents a single inline element in a Lark post paragraph.
type postElement = map[string]interface{}

// markdownToPost converts markdown text into Lark post content paragraphs.
// The returned structure is suitable for the "content" field under "zh_cn".
func markdownToPost(md string) [][]postElement {
	if md == "" {
		return nil
	}

	exts := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock |
		parser.Strikethrough | parser.FencedCode | parser.Autolink | parser.Tables
	p := parser.NewWithExtensions(exts)
	doc := p.Parse([]byte(md))

	b := &postBuilder{}
	b.renderNode(doc)
	b.flushParagraph()
	return b.paragraphs
}

type postBuilder struct {
	paragraphs [][]postElement
	current    []postElement
	styles     []string // style stack for nested formatting
}

func (b *postBuilder) flushParagraph() {
	if len(b.current) > 0 {
		b.paragraphs = append(b.paragraphs, b.current)
		b.current = nil
	}
}

func (b *postBuilder) addText(text string) {
	if text == "" {
		return
	}
	el := postElement{"tag": "text", "text": text}
	if styles := b.deduplicatedStyles(); len(styles) > 0 {
		el["style"] = styles
	}
	b.current = append(b.current, el)
}

func (b *postBuilder) addLink(text, href string) {
	b.current = append(b.current, postElement{"tag": "a", "text": text, "href": href})
}

func (b *postBuilder) deduplicatedStyles() []string {
	if len(b.styles) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range b.styles {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (b *postBuilder) pushStyle(s string) { b.styles = append(b.styles, s) }
func (b *postBuilder) popStyle()           { b.styles = b.styles[:len(b.styles)-1] }

func (b *postBuilder) renderChildren(node ast.Node) {
	for _, child := range node.GetChildren() {
		b.renderNode(child)
	}
}

func (b *postBuilder) renderNode(node ast.Node) {
	switch n := node.(type) {
	case *ast.Document:
		b.renderChildren(node)

	case *ast.Paragraph:
		b.renderChildren(node)
		b.flushParagraph()

	case *ast.Heading:
		b.pushStyle("bold")
		b.renderChildren(node)
		b.popStyle()
		b.flushParagraph()

	case *ast.BlockQuote:
		b.pushStyle("italic")
		b.renderChildren(node)
		b.popStyle()

	case *ast.Strong:
		b.pushStyle("bold")
		b.renderChildren(node)
		b.popStyle()

	case *ast.Emph:
		b.pushStyle("italic")
		b.renderChildren(node)
		b.popStyle()

	case *ast.Del:
		b.pushStyle("lineThrough")
		b.renderChildren(node)
		b.popStyle()

	case *ast.Code:
		// Inline code — Lark post has no inline code style; render as underline.
		b.pushStyle("underline")
		b.addText(string(n.Literal))
		b.popStyle()

	case *ast.CodeBlock:
		b.flushParagraph()
		lang := codeLang(string(n.Info))
		code := strings.TrimRight(string(n.Literal), "\n")
		el := postElement{"tag": "code_block", "text": code}
		if lang != "" {
			el["language"] = lang
		}
		b.paragraphs = append(b.paragraphs, []postElement{el})

	case *ast.Link:
		text := collectLinkText(node)
		if text == "" {
			text = string(n.Destination)
		}
		b.addLink(text, string(n.Destination))

	case *ast.Text:
		b.addText(string(n.Literal))

	case *ast.Softbreak, *ast.Hardbreak:
		b.addText("\n")

	case *ast.HorizontalRule:
		b.flushParagraph()
		b.paragraphs = append(b.paragraphs, []postElement{{"tag": "hr"}})

	case *ast.List:
		b.renderList(n)

	case *ast.Table:
		b.renderTable(n)

	case *ast.HTMLBlock:
		b.addText(strings.TrimRight(string(n.Literal), "\n"))
		b.flushParagraph()

	case *ast.HTMLSpan:
		b.addText(string(n.Literal))

	default:
		if len(node.GetChildren()) > 0 {
			b.renderChildren(node)
			return
		}
		if leaf := node.AsLeaf(); leaf != nil && len(leaf.Literal) > 0 {
			b.addText(string(leaf.Literal))
		}
	}
}

func (b *postBuilder) renderList(list *ast.List) {
	ordered := list.ListFlags&ast.ListTypeOrdered != 0
	index := list.Start
	if index <= 0 {
		index = 1
	}

	for _, child := range list.GetChildren() {
		item, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}

		if ordered {
			b.addText(strconv.Itoa(index) + ". ")
			index++
		} else {
			b.addText("• ")
		}

		for _, ic := range item.GetChildren() {
			if p, ok := ic.(*ast.Paragraph); ok {
				b.renderChildren(p)
			} else {
				b.renderNode(ic)
			}
		}
		b.flushParagraph()
	}
}

func (b *postBuilder) renderTable(table *ast.Table) {
	for _, child := range table.GetChildren() {
		switch child.(type) {
		case *ast.TableHeader, *ast.TableBody:
			for _, row := range child.GetChildren() {
				tr, ok := row.(*ast.TableRow)
				if !ok {
					continue
				}
				cells := tr.GetChildren()
				for i, cell := range cells {
					if i > 0 {
						b.addText(" | ")
					}
					b.renderChildren(cell)
				}
				b.flushParagraph()
			}
		}
	}
}

func collectLinkText(node ast.Node) string {
	var sb strings.Builder
	collectLinkTextInner(node, &sb)
	return sb.String()
}

func collectLinkTextInner(node ast.Node, sb *strings.Builder) {
	if t, ok := node.(*ast.Text); ok {
		sb.Write(t.Literal)
	}
	for _, child := range node.GetChildren() {
		collectLinkTextInner(child, sb)
	}
}

func codeLang(info string) string {
	if f := strings.Fields(info); len(f) > 0 {
		return f[0]
	}
	return ""
}
