#!/usr/bin/env bash
# Build a throwaway git repo so the demo tapes have representative content to
# show: a few commits (for `log`), staged + unstaged changes (for `diff` /
# `stage`), and a stash (for `stash`). Deterministic — fixed commit dates — so
# the captured screenshots stay stable across runs.
set -euo pipefail

DEMO="${1:-/tmp/rift-demo}"
rm -rf "$DEMO"
mkdir -p "$DEMO"
cd "$DEMO"

git init -q -b main
git config user.name "Ada Lovelace"
git config user.email "ada@example.com"

commit() { GIT_AUTHOR_DATE="$1" GIT_COMMITTER_DATE="$1" git commit -q -m "$2"; }

# ── commit 1 ───────────────────────────────────────────────────────────────
mkdir -p internal/parse
cat > internal/parse/lexer.go <<'EOF'
package parse

// Lexer turns source bytes into a token stream.
type Lexer struct {
	src []byte
	pos int
}

func New(src []byte) *Lexer {
	return &Lexer{src: src}
}

func (l *Lexer) Next() (Token, bool) {
	if l.pos >= len(l.src) {
		return Token{}, false
	}
	c := l.src[l.pos]
	l.pos++
	return Token{Kind: classify(c), Value: string(c)}, true
}
EOF
cat > README.md <<'EOF'
# nibble

A tiny, fast tokenizer.

## Usage

    lex := parse.New(src)
    for {
        tok, ok := lex.Next()
        if !ok {
            break
        }
    }
EOF
echo '{"name":"nibble","version":"0.1.0"}' > package.json
git add -A
commit "2026-05-20T10:00:00" "Initial lexer"

# ── commit 2 ───────────────────────────────────────────────────────────────
cat > internal/parse/token.go <<'EOF'
package parse

type Kind int

const (
	Ident Kind = iota
	Number
	Symbol
)

type Token struct {
	Kind  Kind
	Value string
}

func classify(c byte) Kind {
	switch {
	case c >= '0' && c <= '9':
		return Number
	case c >= 'a' && c <= 'z':
		return Ident
	default:
		return Symbol
	}
}
EOF
git add -A
commit "2026-05-22T14:30:00" "Add token classification"

# ── commit 3 ───────────────────────────────────────────────────────────────
echo '{"name":"nibble","version":"0.2.0"}' > package.json
git add -A
commit "2026-05-25T09:15:00" "Bump to 0.2.0"

# ── working tree: a stash, a staged file, and unstaged edits ────────────────
# stash a throwaway experiment so `rift stash` has an entry.
cat >> internal/parse/lexer.go <<'EOF'

func (l *Lexer) Peek() byte { return l.src[l.pos] }
EOF
git stash -q -m "experiment: Peek()"

# staged: a new file.
cat > CONTRIBUTING.md <<'EOF'
# Contributing

Run `go test ./...` before opening a PR.
EOF
git add CONTRIBUTING.md

# unstaged: a refactor (skip whitespace) — a nice structural diff.
cat > internal/parse/lexer.go <<'EOF'
package parse

import "unicode"

// Lexer turns source bytes into a token stream, skipping whitespace.
type Lexer struct {
	src []byte
	pos int
}

func New(src []byte) *Lexer {
	return &Lexer{src: src}
}

func (l *Lexer) Next() (Token, bool) {
	for l.pos < len(l.src) && unicode.IsSpace(rune(l.src[l.pos])) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		return Token{}, false
	}
	c := l.src[l.pos]
	l.pos++
	return Token{Kind: classify(c), Value: string(c)}, true
}
EOF
echo '{"name":"nibble","version":"0.2.0","license":"MIT"}' > package.json

echo "demo repo ready at $DEMO"
