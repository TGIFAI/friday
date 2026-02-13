---
name: chinese-copywriting
description: Edit and normalize Chinese copywriting (especially mixed Chinese-English text) using Chinese Copywriting Guidelines conventions. Use when proofreading, rewriting, or linting Chinese text for spacing, punctuation width, capitalization of proper nouns, and abbreviation quality.
---

# Chinese Copywriting

Apply consistent Chinese copywriting rules while preserving the original meaning and tone.

## Defaults

- Prefer Simplified Chinese and Mainland punctuation style unless the user requests another locale.
- Preserve facts, structure, and intent.
- Apply minimal edits when the user asks for proofreading/normalization only.

## Workflow

1. Detect target language/style (Simplified Chinese by default).
2. Apply hard rules first (spacing, punctuation width, capitalization).
3. Review proper nouns and abbreviation quality.
4. Mark disputed style choices as optional items.
5. Return the revised text and a concise change summary.

## Hard Rules

### Spacing

- Insert one space between Chinese and Latin words.
  - Example: `在 LeanCloud 上，数据存储是围绕 AVObject 进行的。`
- Insert one space between Chinese and Arabic numerals.
  - Example: `今天出去买菜花了 5000 元。`
- Insert one space between numbers and units/symbol words.
  - Example: `我家的光纤入户宽带有 10 Gbps，SSD 一共有 20 TB。`
- Do not insert spaces between numbers and `%` or `°`.
  - Example: `我今天有 15% 的电量，还能跑 90°。`
- Do not insert spaces around full-width punctuation.
  - Example: `刚刚买了一部 iPhone，好开心！`

### Punctuation and Width

- Use full-width Chinese punctuation in Chinese sentences.
  - Example: `嗨！你知道嘛？今天前台的小妹跟我说“喵”了哎！`
- Keep half-width punctuation inside fully English sentences.
  - Example: `Stay hungry, stay foolish.`
- Use half-width Arabic numerals by default.
  - Example: `这件蛋糕只卖 1000 元。`
- Avoid repeated punctuation such as `！！`, `？？`, `？！？？`.

### Proper Nouns and Abbreviations

- Use official capitalization for product and brand names.
  - Example: `iPhone`, `App Store`, `GitHub`, `TypeScript`, `Next.js`.
- Prefer complete, standard terms over non-standard abbreviations.
  - Replace patterns like `APP` (when meaning application UI copy), `h5`, `RJS`, `TS` misuse when context indicates nonstandard shorthand.
- Keep established Chinese product naming forms if they are official names.
  - Example: `豆瓣FM` may remain unchanged if that is the official product name in context.

## Optional/Controversial Items

- Quote style (`“”` vs `「」`) can vary by locale. Keep Mainland default as `“”` unless asked otherwise.
- Some spacing/wording conventions differ across teams; if uncertain, keep the original and flag it as optional.

## Output Format

When asked to proofread or revise:

1. `修订稿` (full revised text)
2. `修改说明` (short bullets grouped by rule type)
3. `可选项` (only when non-mandatory style choices exist)

When asked to "only fix formatting":

- Apply only spacing, punctuation, width, and capitalization rules.
- Do not rewrite semantics or tone.

## Quick Checklist

- `中英之间空格`
- `中文与数字之间空格`
- `数字与单位之间空格（% / ° 例外）`
- `中文标点全角`
- `英文句内标点半角`
- `数字使用半角`
- `专有名词大小写正确`
- `避免不地道缩写`

## Reference

- <https://github.com/sparanoid/chinese-copywriting-guidelines>
- <https://raw.githubusercontent.com/sparanoid/chinese-copywriting-guidelines/refs/heads/master/README.zh-Hans.md>
