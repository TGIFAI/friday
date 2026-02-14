package telegram

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

func convertMarkdownEntities(md string) (string, []models.MessageEntity) {
	if md == "" {
		return "", nil
	}

	exts := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock |
		parser.Strikethrough | parser.FencedCode | parser.Autolink | parser.Tables
	mdParser := parser.NewWithExtensions(exts)
	doc := mdParser.Parse([]byte(md))

	builder := &entityRenderState{}
	renderMarkdownEntityNode(doc, builder)
	builder.normalizeEntities()

	return builder.String(), builder.entities
}

type entityRenderState struct {
	text     strings.Builder
	offset16 int
	entities []models.MessageEntity
}

func (s *entityRenderState) String() string {
	return s.text.String()
}

func (s *entityRenderState) writeString(v string) {
	if v == "" {
		return
	}
	s.text.WriteString(v)
	s.offset16 += utf16Length(v)
}

func (s *entityRenderState) writeByte(v byte) {
	s.writeString(string(v))
}

func (s *entityRenderState) addEntity(
	entityType models.MessageEntityType,
	startOffset16 int,
	url string,
	language string,
) {
	length := s.offset16 - startOffset16
	if length <= 0 {
		return
	}

	entity := models.MessageEntity{
		Type:   entityType,
		Offset: startOffset16,
		Length: length,
	}
	if url != "" {
		entity.URL = url
	}
	if language != "" {
		entity.Language = language
	}

	s.entities = append(s.entities, entity)
}

func (s *entityRenderState) normalizeEntities() {
	if len(s.entities) <= 1 {
		return
	}

	sort.SliceStable(s.entities, func(i, j int) bool {
		if s.entities[i].Offset != s.entities[j].Offset {
			return s.entities[i].Offset < s.entities[j].Offset
		}
		return s.entities[i].Length > s.entities[j].Length
	})
}

func renderMarkdownEntityChildren(node ast.Node, state *entityRenderState) {
	for _, child := range node.GetChildren() {
		renderMarkdownEntityNode(child, state)
	}
}

func renderMarkdownEntityNode(node ast.Node, state *entityRenderState) {
	switch n := node.(type) {
	case *ast.Document:
		renderMarkdownEntityChildren(node, state)
	case *ast.Paragraph:
		renderMarkdownEntityChildren(node, state)
		if ast.GetNextNode(node) != nil {
			if _, ok := node.GetParent().(*ast.ListItem); ok {
				state.writeByte('\n')
			} else {
				state.writeString("\n\n")
			}
		}
	case *ast.Heading:
		start := state.offset16
		renderMarkdownEntityChildren(node, state)
		state.addEntity(models.MessageEntityTypeBold, start, "", "")
		if ast.GetNextNode(node) != nil {
			state.writeString("\n\n")
		}
	case *ast.BlockQuote:
		start := state.offset16
		renderMarkdownEntityChildren(node, state)
		state.addEntity(models.MessageEntityTypeBlockquote, start, "", "")
		if ast.GetNextNode(node) != nil {
			state.writeString("\n\n")
		}
	case *ast.List:
		renderMarkdownEntityList(n, state)
		if ast.GetNextNode(node) != nil {
			state.writeString("\n\n")
		}
	case *ast.ListItem:
		renderMarkdownEntityListItem(n, state)
	case *ast.Strong:
		start := state.offset16
		renderMarkdownEntityChildren(node, state)
		state.addEntity(models.MessageEntityTypeBold, start, "", "")
	case *ast.Emph:
		start := state.offset16
		renderMarkdownEntityChildren(node, state)
		state.addEntity(models.MessageEntityTypeItalic, start, "", "")
	case *ast.Del:
		start := state.offset16
		renderMarkdownEntityChildren(node, state)
		state.addEntity(models.MessageEntityTypeStrikethrough, start, "", "")
	case *ast.Code:
		start := state.offset16
		state.writeString(string(n.Literal))
		state.addEntity(models.MessageEntityTypeCode, start, "", "")
	case *ast.CodeBlock:
		start := state.offset16
		code := strings.TrimRight(string(n.Literal), "\n")
		state.writeString(code)
		state.addEntity(models.MessageEntityTypePre, start, "", codeLang(string(n.Info)))
		if ast.GetNextNode(node) != nil {
			state.writeString("\n\n")
		}
	case *ast.Link:
		start := state.offset16
		renderMarkdownEntityChildren(node, state)
		if state.offset16 > start {
			state.addEntity(models.MessageEntityTypeTextLink, start, string(n.Destination), "")
		} else {
			state.writeString(string(n.Destination))
		}
	case *ast.Text:
		state.writeString(string(n.Literal))
	case *ast.Softbreak, *ast.Hardbreak:
		state.writeByte('\n')
	case *ast.HorizontalRule:
		state.writeString(strings.Repeat("-", 10))
		if ast.GetNextNode(node) != nil {
			state.writeString("\n\n")
		}
	case *ast.HTMLBlock:
		state.writeString(string(n.Literal))
		if ast.GetNextNode(node) != nil {
			state.writeString("\n\n")
		}
	case *ast.HTMLSpan:
		state.writeString(string(n.Literal))
	default:
		if len(node.GetChildren()) > 0 {
			renderMarkdownEntityChildren(node, state)
			return
		}

		leaf := node.AsLeaf()
		if leaf != nil && len(leaf.Literal) > 0 {
			state.writeString(string(leaf.Literal))
		}
	}
}

func renderMarkdownEntityList(list *ast.List, state *entityRenderState) {
	ordered := list.ListFlags&ast.ListTypeOrdered != 0
	index := list.Start
	if index <= 0 {
		index = 1
	}

	items := list.GetChildren()
	for i, one := range items {
		item, ok := one.(*ast.ListItem)
		if !ok {
			continue
		}

		if ordered {
			state.writeString(strconv.Itoa(index))
			state.writeString(". ")
			index++
		} else {
			state.writeString("- ")
		}

		renderMarkdownEntityListItem(item, state)
		if i < len(items)-1 {
			state.writeByte('\n')
		}
	}
}

func renderMarkdownEntityListItem(item *ast.ListItem, state *entityRenderState) {
	children := item.GetChildren()
	for i, child := range children {
		if paragraph, ok := child.(*ast.Paragraph); ok {
			renderMarkdownEntityChildren(paragraph, state)
		} else {
			renderMarkdownEntityNode(child, state)
		}

		if i < len(children)-1 {
			state.writeByte('\n')
		}
	}
}

func utf16Length(input string) int {
	return len(utf16.Encode([]rune(input)))
}

func codeLang(info string) string {
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
