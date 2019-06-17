package protoparse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

type runeReader struct {
	rr     *bufio.Reader
	unread []rune
	err    error
}

func (rr *runeReader) readRune() (r rune, size int, err error) {
	if rr.err != nil {
		return 0, 0, rr.err
	}
	if len(rr.unread) > 0 {
		r := rr.unread[len(rr.unread)-1]
		rr.unread = rr.unread[:len(rr.unread)-1]
		return r, utf8.RuneLen(r), nil
	}
	r, sz, err := rr.rr.ReadRune()
	if err != nil {
		rr.err = err
	}
	return r, sz, err
}

func (rr *runeReader) unreadRune(r rune) {
	rr.unread = append(rr.unread, r)
}

func lexError(l protoLexer, pos *SourcePos, err string) {
	pl := l.(*protoLex)
	if pl.err == nil {
		pl.err = ErrorWithSourcePos{Underlying: errors.New(err), Pos: pos}
	}
}

type protoLex struct {
	filename string
	input    *runeReader
	err      error
	res      *fileNode

	lineNo int
	colNo  int
	offset int

	prevSym terminalNode
}

func newLexer(in io.Reader) *protoLex {
	return &protoLex{input: &runeReader{rr: bufio.NewReader(in)}}
}

var keywords = map[string]int{
	"syntax":     _SYNTAX,
	"import":     _IMPORT,
	"weak":       _WEAK,
	"public":     _PUBLIC,
	"package":    _PACKAGE,
	"option":     _OPTION,
	"true":       _TRUE,
	"false":      _FALSE,
	"inf":        _INF,
	"nan":        _NAN,
	"repeated":   _REPEATED,
	"optional":   _OPTIONAL,
	"required":   _REQUIRED,
	"double":     _DOUBLE,
	"float":      _FLOAT,
	"int32":      _INT32,
	"int64":      _INT64,
	"uint32":     _UINT32,
	"uint64":     _UINT64,
	"sint32":     _SINT32,
	"sint64":     _SINT64,
	"fixed32":    _FIXED32,
	"fixed64":    _FIXED64,
	"sfixed32":   _SFIXED32,
	"sfixed64":   _SFIXED64,
	"bool":       _BOOL,
	"string":     _STRING,
	"bytes":      _BYTES,
	"group":      _GROUP,
	"oneof":      _ONEOF,
	"map":        _MAP,
	"extensions": _EXTENSIONS,
	"to":         _TO,
	"max":        _MAX,
	"reserved":   _RESERVED,
	"enum":       _ENUM,
	"message":    _MESSAGE,
	"extend":     _EXTEND,
	"service":    _SERVICE,
	"rpc":        _RPC,
	"stream":     _STREAM,
	"returns":    _RETURNS,
}

func (l *protoLex) cur() *SourcePos {
	return &SourcePos{
		Filename: l.filename,
		Offset:   l.offset,
		Line:     l.lineNo + 1,
		Col:      l.colNo + 1,
	}
}

func (l *protoLex) prev() *SourcePos {
	if l.prevSym == nil {
		return &SourcePos{
			Filename: l.filename,
			Offset:   0,
			Line:     1,
			Col:      1,
		}
	}
	return l.prevSym.start()
}

func (l *protoLex) Lex(lval *protoSymType) int {
	if l.err != nil {
		// if we are already in a failed state, bail
		lval.err = l.err
		return _ERROR
	}

	prevLineNo := l.lineNo
	prevColNo := l.colNo
	prevOffset := l.offset
	var comments []*comment

	pos := func() posRange {
		return posRange{
			start: &SourcePos{
				Filename: l.filename,
				Offset:   prevOffset,
				Line:     prevLineNo + 1,
				Col:      prevColNo + 1,
			},
			end: l.cur(),
		}
	}
	basic := func() basicNode {
		return basicNode{
			posRange: pos(),
			leading:  comments,
		}
	}
	setPrev := func(n terminalNode) {
		nStart := n.start().Line
		if _, ok := n.(*basicNode); ok {
			// if the node is a simple rune, don't attribute comments to it
			// HACK: adjusting the start line makes leading comments appear
			// detached so logic below will naturally associated trailing
			// comment to previous symbol
			nStart += 2
		}
		if l.prevSym != nil && len(n.leadingComments()) > 0 && l.prevSym.end().Line < nStart {
			// we may need to re-attribute the first comment to
			// instead be previous node's trailing comment
			prevEnd := l.prevSym.end().Line
			comments := n.leadingComments()
			c := comments[0]
			commentStart := c.start.Line
			if commentStart == prevEnd {
				// comment is on same line as previous symbol
				n.popLeadingComment()
				l.prevSym.pushTrailingComment(c)
			} else if commentStart == prevEnd+1 {
				// comment is right after previous symbol; see if it is detached
				// and if so re-attribute
				singleLineStyle := strings.HasPrefix(c.text, "//")
				line := c.end.Line
				groupEnd := -1
				for i := 1; i < len(comments); i++ {
					c := comments[i]
					newGroup := false
					if !singleLineStyle || c.start.Line > line+1 {
						// we've found a gap between comments, which means the
						// previous comments were detached
						newGroup = true
					} else {
						line = c.end.Line
						singleLineStyle = strings.HasPrefix(comments[i].text, "//")
						if !singleLineStyle {
							// we've found a switch from // comments to /*
							// consider that a new group which means the
							// previous comments were detached
							newGroup = true
						}
					}
					if newGroup {
						groupEnd = i
						break
					}
				}

				if groupEnd == -1 {
					// just one group of comments; we'll mark it as a trailing
					// comment if it immediately follows previous symbol and is
					// detached from current symbol
					c1 := comments[0]
					c2 := comments[len(comments)-1]
					if c1.start.Line <= prevEnd+1 && c2.end.Line < nStart-1 {
						groupEnd = len(comments)
					}
				}

				for i := 0; i < groupEnd; i++ {
					l.prevSym.pushTrailingComment(n.popLeadingComment())
				}
			}
		}

		l.prevSym = n
	}
	setString := func(val string) {
		b := basic()
		lval.str = &stringLiteralNode{val: val}
		lval.str.setRange(&b, &b)
		setPrev(lval.str)
	}
	setIdent := func(val string, kind identKind) {
		b := basic()
		lval.id = &identNode{val: val, kind: kind}
		lval.id.setRange(&b, &b)
		setPrev(lval.id)
	}
	setInt := func(val uint64) {
		lval.ui = &intLiteralNode{basicNode: basic(), val: val}
		setPrev(lval.ui)
	}
	setFloat := func(val float64) {
		b := basic()
		lval.f = &floatLiteralNode{val: val}
		lval.f.setRange(&b, &b)
		setPrev(lval.f)
	}
	setRune := func() {
		b := basic()
		lval.b = &b
		setPrev(lval.b)
	}
	setError := func(err error) {
		lval.err = err
		l.err = err
	}

	for {
		c, n, err := l.input.readRune()
		if err == io.EOF {
			// we're not actually returning a rune, but this will associate
			// accumulated comments as a trailing comment on last symbol
			// (if appropriate)
			setRune()
			return 0
		} else if err != nil {
			setError(err)
			return _ERROR
		}

		prevLineNo = l.lineNo
		prevColNo = l.colNo
		prevOffset = l.offset

		l.offset += n
		if c == '\n' {
			l.colNo = 0
			l.lineNo++
			continue
		} else if c == '\r' {
			continue
		}
		l.colNo++
		if c == ' ' || c == '\t' {
			continue
		}

		if c == '.' {
			// tokens that start with a dot include type names and decimal literals
			cn, _, err := l.input.readRune()
			if err != nil {
				setRune()
				return int(c)
			}
			if cn == '_' || (cn >= 'a' && cn <= 'z') || (cn >= 'A' && cn <= 'Z') {
				l.colNo++
				token := []rune{c, cn}
				token = l.readIdentifier(token)
				setIdent(string(token), identTypeName)
				return _TYPENAME
			}
			if cn >= '0' && cn <= '9' {
				l.colNo++
				token := []rune{c, cn}
				token = l.readNumber(token, false, true)
				f, err := strconv.ParseFloat(string(token), 64)
				if err != nil {
					setError(err)
					return _ERROR
				}
				setFloat(f)
				return _FLOAT_LIT
			}
			l.input.unreadRune(cn)
			setRune()
			return int(c)
		}

		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			// identifier
			token := []rune{c}
			token = l.readIdentifier(token)
			str := string(token)
			if strings.Contains(str, ".") {
				setIdent(str, identQualified)
				return _FQNAME
			}
			if t, ok := keywords[str]; ok {
				setIdent(str, identSimpleName)
				return t
			}
			setIdent(str, identSimpleName)
			return _NAME
		}

		if c >= '0' && c <= '9' {
			// integer or float literal
			if c == '0' {
				cn, _, err := l.input.readRune()
				if err != nil {
					setInt(0)
					return _INT_LIT
				}
				if cn == 'x' || cn == 'X' {
					cnn, _, err := l.input.readRune()
					if err != nil {
						l.input.unreadRune(cn)
						setInt(0)
						return _INT_LIT
					}
					if (cnn >= '0' && cnn <= '9') || (cnn >= 'a' && cnn <= 'f') || (cnn >= 'A' && cnn <= 'F') {
						// hexadecimal!
						l.colNo += 2
						token := []rune{cnn}
						token = l.readHexNumber(token)
						ui, err := strconv.ParseUint(string(token), 16, 64)
						if err != nil {
							setError(err)
							return _ERROR
						}
						setInt(ui)
						return _INT_LIT
					}
					l.input.unreadRune(cnn)
					l.input.unreadRune(cn)
					setInt(0)
					return _INT_LIT
				} else {
					l.input.unreadRune(cn)
				}
			}
			token := []rune{c}
			token = l.readNumber(token, true, true)
			numstr := string(token)
			if strings.Contains(numstr, ".") || strings.Contains(numstr, "e") || strings.Contains(numstr, "E") {
				// floating point!
				f, err := strconv.ParseFloat(numstr, 64)
				if err != nil {
					setError(err)
					return _ERROR
				}
				setFloat(f)
				return _FLOAT_LIT
			}
			// integer! (decimal or octal)
			ui, err := strconv.ParseUint(numstr, 0, 64)
			if err != nil {
				setError(err)
				return _ERROR
			}
			setInt(ui)
			return _INT_LIT
		}

		if c == '\'' || c == '"' {
			// string literal
			str, err := l.readStringLiteral(c)
			if err != nil {
				setError(err)
				return _ERROR
			}
			setString(str)
			return _STRING_LIT
		}

		if c == '/' {
			// comment
			cn, _, err := l.input.readRune()
			if err != nil {
				setRune()
				return int(c)
			}
			if cn == '/' {
				l.colNo++
				hitNewline, txt := l.skipToEndOfLineComment()
				commentPos := pos()
				commentPos.end.Col++
				if hitNewline {
					l.colNo = 0
					l.lineNo++
				}
				comments = append(comments, &comment{posRange: commentPos, text: txt})
				continue
			}
			if cn == '*' {
				l.colNo++
				if txt, ok := l.skipToEndOfBlockComment(); !ok {
					setError(errors.New("block comment never terminates, unexpected EOF"))
					return _ERROR
				} else {
					comments = append(comments, &comment{posRange: pos(), text: txt})
				}
				continue
			}
			l.input.unreadRune(cn)
		}

		setRune()
		return int(c)
	}
}

func (l *protoLex) readNumber(sofar []rune, allowDot bool, allowExp bool) []rune {
	token := sofar
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			break
		}
		if c == '.' {
			if !allowDot {
				l.input.unreadRune(c)
				break
			}
			allowDot = false
			cn, _, err := l.input.readRune()
			if err != nil {
				l.input.unreadRune(c)
				break
			}
			if cn < '0' || cn > '9' {
				l.input.unreadRune(cn)
				l.input.unreadRune(c)
				break
			}
			l.colNo++
			token = append(token, c)
			c = cn
		} else if c == 'e' || c == 'E' {
			if !allowExp {
				l.input.unreadRune(c)
				break
			}
			allowExp = false
			cn, _, err := l.input.readRune()
			if err != nil {
				l.input.unreadRune(c)
				break
			}
			if cn == '-' || cn == '+' {
				cnn, _, err := l.input.readRune()
				if err != nil {
					l.input.unreadRune(cn)
					l.input.unreadRune(c)
					break
				}
				if cnn < '0' || cnn > '9' {
					l.input.unreadRune(cnn)
					l.input.unreadRune(cn)
					l.input.unreadRune(c)
					break
				}
				l.colNo++
				token = append(token, c)
				c = cn
				cn = cnn
			} else if cn < '0' || cn > '9' {
				l.input.unreadRune(cn)
				l.input.unreadRune(c)
				break
			}
			l.colNo++
			token = append(token, c)
			c = cn
		} else if c < '0' || c > '9' {
			l.input.unreadRune(c)
			break
		}
		l.colNo++
		token = append(token, c)
	}
	return token
}

func (l *protoLex) readHexNumber(sofar []rune) []rune {
	token := sofar
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			break
		}
		if (c < 'a' || c > 'f') && (c < 'A' || c > 'F') && (c < '0' || c > '9') {
			l.input.unreadRune(c)
			break
		}
		l.colNo++
		token = append(token, c)
	}
	return token
}

func (l *protoLex) readIdentifier(sofar []rune) []rune {
	token := sofar
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			break
		}
		if c == '.' {
			cn, _, err := l.input.readRune()
			if err != nil {
				l.input.unreadRune(c)
				break
			}
			if cn != '_' && (cn < 'a' || cn > 'z') && (cn < 'A' || cn > 'Z') {
				l.input.unreadRune(cn)
				l.input.unreadRune(c)
				break
			}
			l.colNo++
			token = append(token, c)
			c = cn
		} else if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			l.input.unreadRune(c)
			break
		}
		l.colNo++
		token = append(token, c)
	}
	return token
}

func (l *protoLex) readStringLiteral(quote rune) (string, error) {
	var buf bytes.Buffer
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return "", err
		}
		if c == '\n' {
			l.colNo = 0
			l.lineNo++
			return "", errors.New("encountered end-of-line before end of string literal")
		}
		l.colNo++
		if c == quote {
			break
		}
		if c == 0 {
			return "", errors.New("null character ('\\0') not allowed in string literal")
		}
		if c == '\\' {
			// escape sequence
			c, _, err = l.input.readRune()
			if err != nil {
				return "", err
			}
			l.colNo++
			if c == 'x' || c == 'X' {
				// hex escape
				c, _, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				l.colNo++
				c2, _, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				var hex string
				if (c2 < '0' || c2 > '9') && (c2 < 'a' || c2 > 'f') && (c2 < 'A' || c2 > 'F') {
					l.input.unreadRune(c2)
					hex = string(c)
				} else {
					l.colNo++
					hex = string([]rune{c, c2})
				}
				i, err := strconv.ParseInt(hex, 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid hex escape: \\x%q", hex)
				}
				buf.WriteByte(byte(i))

			} else if c >= '0' && c <= '7' {
				// octal escape
				c2, _, err := l.input.readRune()
				if err != nil {
					return "", err
				}
				var octal string
				if c2 < '0' || c2 > '7' {
					l.input.unreadRune(c2)
					octal = string(c)
				} else {
					l.colNo++
					c3, _, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					if c3 < '0' || c3 > '7' {
						l.input.unreadRune(c3)
						octal = string([]rune{c, c2})
					} else {
						l.colNo++
						octal = string([]rune{c, c2, c3})
					}
				}
				i, err := strconv.ParseInt(octal, 8, 32)
				if err != nil {
					return "", fmt.Errorf("invalid octal escape: \\%q", octal)
				}
				if i > 0xff {
					return "", fmt.Errorf("octal escape is out range, must be between 0 and 377: \\%q", octal)
				}
				buf.WriteByte(byte(i))

			} else if c == 'u' {
				// short unicode escape
				u := make([]rune, 4)
				for i := range u {
					c, _, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					l.colNo++
					u[i] = c
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape: \\u%q", string(u))
				}
				buf.WriteRune(rune(i))

			} else if c == 'U' {
				// long unicode escape
				u := make([]rune, 8)
				for i := range u {
					c, _, err := l.input.readRune()
					if err != nil {
						return "", err
					}
					l.colNo++
					u[i] = c
				}
				i, err := strconv.ParseInt(string(u), 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape: \\U%q", string(u))
				}
				if i > 0x10ffff || i < 0 {
					return "", fmt.Errorf("unicode escape is out of range, must be between 0 and 0x10ffff: \\U%q", string(u))
				}
				buf.WriteRune(rune(i))

			} else if c == 'a' {
				buf.WriteByte('\a')
			} else if c == 'b' {
				buf.WriteByte('\b')
			} else if c == 'f' {
				buf.WriteByte('\f')
			} else if c == 'n' {
				buf.WriteByte('\n')
			} else if c == 'r' {
				buf.WriteByte('\r')
			} else if c == 't' {
				buf.WriteByte('\t')
			} else if c == 'v' {
				buf.WriteByte('\v')
			} else if c == '\\' {
				buf.WriteByte('\\')
			} else if c == '\'' {
				buf.WriteByte('\'')
			} else if c == '"' {
				buf.WriteByte('"')
			} else if c == '?' {
				buf.WriteByte('?')
			} else {
				return "", fmt.Errorf("invalid escape sequence: %q", "\\"+string(c))
			}
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String(), nil
}

func (l *protoLex) skipToEndOfLineComment() (bool, string) {
	txt := []rune{'/', '/'}
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return false, string(txt)
		}
		if c == '\n' {
			return true, string(txt)
		}
		l.colNo++
		txt = append(txt, c)
	}
}

func (l *protoLex) skipToEndOfBlockComment() (string, bool) {
	txt := []rune{'/', '*'}
	for {
		c, _, err := l.input.readRune()
		if err != nil {
			return "", false
		}
		if c == '\n' {
			l.colNo = 0
			l.lineNo++
		} else {
			l.colNo++
		}
		txt = append(txt, c)
		if c == '*' {
			c, _, err := l.input.readRune()
			if err != nil {
				return "", false
			}
			if c == '/' {
				l.colNo++
				txt = append(txt, c)
				return string(txt), true
			}
			l.input.unreadRune(c)
		}
	}
}

func (l *protoLex) Error(s string) {
	if l.err == nil {
		l.err = ErrorWithSourcePos{Underlying: errors.New(s), Pos: l.prevSym.start()}
	}
}
