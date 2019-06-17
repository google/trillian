%{
package protoparse

//lint:file-ignore SA4006 generated parser has unused values

import (
	"fmt"
	"math"
	"unicode"

	"github.com/jhump/protoreflect/desc/internal"
)

%}

// fields inside this union end up as the fields in a structure known
// as ${PREFIX}SymType, of which a reference is passed to the lexer.
%union{
	file      *fileNode
	fileDecls []*fileElement
	syn       *syntaxNode
	pkg       *packageNode
	imprt     *importNode
	msg       *messageNode
	msgDecls  []*messageElement
	fld       *fieldNode
	mapFld    *mapFieldNode
	grp       *groupNode
	oo        *oneOfNode
	ooDecls   []*oneOfElement
	ext       *extensionRangeNode
	resvd     *reservedNode
	en        *enumNode
	enDecls   []*enumElement
	env       *enumValueNode
	extend    *extendNode
	extDecls  []*extendElement
	svc       *serviceNode
	svcDecls  []*serviceElement
	mtd       *methodNode
	rpcType   *rpcTypeNode
	opts      []*optionNode
	optNm     []*optionNamePartNode
	rngs      []*rangeNode
	names     []*stringLiteralNode
	sl        []valueNode
	agg       []*aggregateEntryNode
	aggName   *aggregateNameNode
	v         valueNode
	str       *stringLiteralNode
	i         *negativeIntLiteralNode
	ui        *intLiteralNode
	f         *floatLiteralNode
	id        *identNode
	b         *basicNode
	err       error
}

// any non-terminal which returns a value needs a type, which is
// really a field name in the above union struct
%type <file>      file
%type <syn>       syntax
%type <fileDecls> fileDecl fileDecls
%type <imprt>     import
%type <pkg>       package
%type <opts>      option fieldOption fieldOptions rpcOption rpcOptions
%type <optNm>     optionName optionNameRest optionNameComponent
%type <v>         constant scalarConstant aggregate
%type <id>        name ident typeIdent keyType
%type <aggName>   aggName
%type <i>         negIntLit
%type <ui>        intLit
%type <f>         floatLit
%type <sl>        constantList
%type <agg>       aggFields aggField aggFieldEntry
%type <fld>       field oneofField
%type <oo>        oneof
%type <grp>       group
%type <mapFld>    mapField
%type <msg>       message
%type <msgDecls>  messageItem messageBody
%type <ooDecls>   oneofItem oneofBody
%type <names>     fieldNames
%type <resvd>     msgReserved enumReserved reservedNames
%type <rngs>      tagRange tagRanges enumRange enumRanges
%type <ext>       extensions
%type <en>        enum
%type <enDecls>   enumItem enumBody
%type <env>       enumField
%type <extend>    extend
%type <extDecls>  extendItem extendBody
%type <str>       stringLit
%type <svc>       service
%type <svcDecls>  serviceItem serviceBody
%type <mtd>       rpc
%type <rpcType>   rpcType

// same for terminals
%token <str> _STRING_LIT
%token <ui>  _INT_LIT
%token <f>   _FLOAT_LIT
%token <id>  _NAME _FQNAME _TYPENAME
%token <id>  _SYNTAX _IMPORT _WEAK _PUBLIC _PACKAGE _OPTION _TRUE _FALSE _INF _NAN _REPEATED _OPTIONAL _REQUIRED
%token <id>  _DOUBLE _FLOAT _INT32 _INT64 _UINT32 _UINT64 _SINT32 _SINT64 _FIXED32 _FIXED64 _SFIXED32 _SFIXED64
%token <id>  _BOOL _STRING _BYTES _GROUP _ONEOF _MAP _EXTENSIONS _TO _MAX _RESERVED _ENUM _MESSAGE _EXTEND
%token <id>  _SERVICE _RPC _STREAM _RETURNS
%token <err> _ERROR
// we define all of these, even ones that aren't used, to improve error messages
// so it shows the unexpected symbol instead of showing "$unk"
%token <b>   '=' ';' ':' '{' '}' '\\' '/' '?' '.' ',' '>' '<' '+' '-' '(' ')' '[' ']' '*' '&' '^' '%' '$' '#' '@' '!' '~' '`'

%%

file : syntax {
		$$ = &fileNode{syntax: $1}
		$$.setRange($1, $1)
		protolex.(*protoLex).res = $$
	}
	| fileDecls  {
		$$ = &fileNode{decls: $1}
		if len($1) > 0 {
			$$.setRange($1[0], $1[len($1)-1])
		}
		protolex.(*protoLex).res = $$
	}
	| syntax fileDecls {
		$$ = &fileNode{syntax: $1, decls: $2}
		var end node
		if len($2) > 0 {
			end = $2[len($2)-1]
		} else {
			end = $1
		}
		$$.setRange($1, end)
		protolex.(*protoLex).res = $$
	}
	| {
	}

fileDecls : fileDecls fileDecl {
		$$ = append($1, $2...)
	}
	| fileDecl

fileDecl : import {
		$$ = []*fileElement{{imp: $1}}
	}
	| package {
		$$ = []*fileElement{{pkg: $1}}
	}
	| option {
		$$ = []*fileElement{{option: $1[0]}}
	}
	| message {
		$$ = []*fileElement{{message: $1}}
	}
	| enum {
		$$ = []*fileElement{{enum: $1}}
	}
	| extend {
		$$ = []*fileElement{{extend: $1}}
	}
	| service {
		$$ = []*fileElement{{service: $1}}
	}
	| ';' {
		$$ = []*fileElement{{empty: $1}}
	}

syntax : _SYNTAX '=' stringLit ';' {
		if $3.val != "proto2" && $3.val != "proto3" {
			lexError(protolex, $3.start(), "syntax value must be 'proto2' or 'proto3'")
		}
		$$ = &syntaxNode{syntax: $3}
		$$.setRange($1, $4)
	}

import : _IMPORT stringLit ';' {
		$$ = &importNode{ name: $2 }
		$$.setRange($1, $3)
	}
	| _IMPORT _WEAK stringLit ';' {
		$$ = &importNode{ name: $3, weak: true }
		$$.setRange($1, $4)
	}
	| _IMPORT _PUBLIC stringLit ';' {
		$$ = &importNode{ name: $3, public: true }
		$$.setRange($1, $4)
	}

package : _PACKAGE ident ';' {
		$$ = &packageNode{name: $2}
		$$.setRange($1, $3)
	}

ident : name
	| _FQNAME

option : _OPTION optionName '=' constant ';' {
		n := &optionNameNode{parts: $2}
		n.setRange($2[0], $2[len($2)-1])
		o := &optionNode{name: n, val: $4}
		o.setRange($1, $5)
		$$ = []*optionNode{o}
	}

optionName : ident {
		$$ = toNameParts($1, 0)
	}
	| '(' typeIdent ')' {
		p := &optionNamePartNode{text: $2, isExtension: true}
		p.setRange($1, $3)
		$$ = []*optionNamePartNode{p}
	}
	| '(' typeIdent ')' optionNameRest {
		p := &optionNamePartNode{text: $2, isExtension: true}
		p.setRange($1, $3)
		ps := make([]*optionNamePartNode, 1, len($4)+1)
		ps[0] = p
		$$ = append(ps, $4...)
	}

optionNameRest : optionNameComponent
	| optionNameComponent optionNameRest {
		$$ = append($1, $2...)
	}

optionNameComponent : _TYPENAME {
		$$ = toNameParts($1, 1 /* exclude leading dot */)
	}
	| '.' '(' typeIdent ')' {
		p := &optionNamePartNode{text: $3, isExtension: true}
		p.setRange($2, $4)
		$$ = []*optionNamePartNode{p}
	}

constant : scalarConstant
	| aggregate

scalarConstant : stringLit {
		$$ = $1
	}
	| intLit {
		$$ = $1
	}
	| negIntLit {
		$$ = $1
	}
	| floatLit {
		$$ = $1
	}
	| name {
		if $1.val == "true" {
			$$ = &boolLiteralNode{basicNode: $1.toBasicNode(), val: true}
		} else if $1.val == "false" {
			$$ = &boolLiteralNode{basicNode: $1.toBasicNode(), val: false}
		} else if $1.val == "inf" {
			f := &floatLiteralNode{val: math.Inf(1)}
			f.setRange($1, $1)
			$$ = f
		} else if $1.val == "nan" {
			f := &floatLiteralNode{val: math.NaN()}
			f.setRange($1, $1)
			$$ = f
		} else {
			$$ = $1
		}
	}

intLit : _INT_LIT
	| '+' _INT_LIT {
		$$ = $2
	}

negIntLit : '-' _INT_LIT {
		if $2.val > math.MaxInt64 + 1 {
			lexError(protolex, $2.start(), fmt.Sprintf("numeric constant %d would underflow (allowed range is %d to %d)", $2.val, int64(math.MinInt64), int64(math.MaxInt64)))
		}
		$$ = &negativeIntLiteralNode{val: -int64($2.val)}
		$$.setRange($1, $2)
	}

floatLit : _FLOAT_LIT
	| '-' _FLOAT_LIT {
		$$ = &floatLiteralNode{val: -$2.val}
		$$.setRange($1, $2)
	}
	| '+' _FLOAT_LIT {
		$$ = &floatLiteralNode{val: $2.val}
		$$.setRange($1, $2)
	}
	| '+' _INF {
		$$ = &floatLiteralNode{val: math.Inf(1)}
		$$.setRange($1, $2)
	}
	| '-' _INF {
		$$ = &floatLiteralNode{val: math.Inf(-1)}
		$$.setRange($1, $2)
	}

stringLit : _STRING_LIT
    | stringLit _STRING_LIT {
        $$ = &stringLiteralNode{val: $1.val + $2.val}
        $$.setRange($1, $2)
    }

aggregate : '{' aggFields '}' {
		a := &aggregateLiteralNode{elements: $2}
		a.setRange($1, $3)
		$$ = a
	}

aggFields : aggField
	| aggFields aggField {
		$$ = append($1, $2...)
	}
	| {
		$$ = nil
	}

aggField : aggFieldEntry
	| aggFieldEntry ',' {
		$$ = $1
	}
	| aggFieldEntry ';' {
		$$ = $1
	}

aggFieldEntry : aggName ':' scalarConstant {
		a := &aggregateEntryNode{name: $1, val: $3}
		a.setRange($1, $3)
		$$ = []*aggregateEntryNode{a}
	}
	| aggName ':' '[' ']' {
		s := &sliceLiteralNode{}
		s.setRange($3, $4)
		a := &aggregateEntryNode{name: $1, val: s}
		a.setRange($1, $4)
		$$ = []*aggregateEntryNode{a}
	}
	| aggName ':' '[' constantList ']' {
		s := &sliceLiteralNode{elements: $4}
		s.setRange($3, $5)
		a := &aggregateEntryNode{name: $1, val: s}
		a.setRange($1, $5)
		$$ = []*aggregateEntryNode{a}
	}
	| aggName ':' aggregate {
		a := &aggregateEntryNode{name: $1, val: $3}
		a.setRange($1, $3)
		$$ = []*aggregateEntryNode{a}
	}
	| aggName aggregate {
		a := &aggregateEntryNode{name: $1, val: $2}
		a.setRange($1, $2)
		$$ = []*aggregateEntryNode{a}
	}
	| aggName ':' '<' aggFields '>' {
		s := &aggregateLiteralNode{elements: $4}
		s.setRange($3, $5)
		a := &aggregateEntryNode{name: $1, val: s}
		a.setRange($1, $5)
		$$ = []*aggregateEntryNode{a}
	}
	| aggName '<' aggFields '>' {
		s := &aggregateLiteralNode{elements: $3}
		s.setRange($2, $4)
		a := &aggregateEntryNode{name: $1, val: s}
		a.setRange($1, $4)
		$$ = []*aggregateEntryNode{a}
	}

aggName : name {
		$$ = &aggregateNameNode{name: $1}
		$$.setRange($1, $1)
	}
	| '[' ident ']' {
		$$ = &aggregateNameNode{name: $2, isExtension: true}
		$$.setRange($1, $3)
	}

constantList : constant {
		$$ = []valueNode{$1}
	}
	| constantList ',' constant {
		$$ = append($1, $3)
	}
	| constantList ';' constant {
		$$ = append($1, $3)
	}
	| '<' aggFields '>' {
		s := &aggregateLiteralNode{elements: $2}
		s.setRange($1, $3)
		$$ = []valueNode{s}
	}
	| constantList ','  '<' aggFields '>' {
		s := &aggregateLiteralNode{elements: $4}
		s.setRange($3, $5)
		$$ = append($1, s)
	}
	| constantList ';'  '<' aggFields '>' {
		s := &aggregateLiteralNode{elements: $4}
		s.setRange($3, $5)
		$$ = append($1, s)
	}

typeIdent : ident
	| _TYPENAME
	| typeIdent _TYPENAME {
        $$ = &identNode{val: $1.val + $2.val}
        $$.setRange($1, $2)
	}

field : _REQUIRED typeIdent name '=' _INT_LIT ';' {
		checkTag(protolex, $5.start(), $5.val)
		lbl := &labelNode{basicNode: $1.toBasicNode(), required: true}
		$$ = &fieldNode{label: lbl, fldType: $2, name: $3, tag: $5}
		$$.setRange($1, $6)
	}
	| _OPTIONAL typeIdent name '=' _INT_LIT ';' {
		checkTag(protolex, $5.start(), $5.val)
		lbl := &labelNode{basicNode: $1.toBasicNode()}
		$$ = &fieldNode{label: lbl, fldType: $2, name: $3, tag: $5}
		$$.setRange($1, $6)
	}
	| _REPEATED typeIdent name '=' _INT_LIT ';' {
		checkTag(protolex, $5.start(), $5.val)
		lbl := &labelNode{basicNode: $1.toBasicNode(), repeated: true}
		$$ = &fieldNode{label: lbl, fldType: $2, name: $3, tag: $5}
		$$.setRange($1, $6)
	}
	| typeIdent name '=' _INT_LIT ';' {
		checkTag(protolex, $4.start(), $4.val)
		$$ = &fieldNode{fldType: $1, name: $2, tag: $4}
		$$.setRange($1, $5)
	}
	| _REQUIRED typeIdent name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkTag(protolex, $5.start(), $5.val)
		lbl := &labelNode{basicNode: $1.toBasicNode(), required: true}
		$$ = &fieldNode{label: lbl, fldType: $2, name: $3, tag: $5, options: $7}
		$$.setRange($1, $9)
	}
	| _OPTIONAL typeIdent name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkTag(protolex, $5.start(), $5.val)
		lbl := &labelNode{basicNode: $1.toBasicNode()}
		$$ = &fieldNode{label: lbl, fldType: $2, name: $3, tag: $5, options: $7}
		$$.setRange($1, $9)
	}
	| _REPEATED typeIdent name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkTag(protolex, $5.start(), $5.val)
		lbl := &labelNode{basicNode: $1.toBasicNode(), repeated: true}
		$$ = &fieldNode{label: lbl, fldType: $2, name: $3, tag: $5, options: $7}
		$$.setRange($1, $9)
	}
	| typeIdent name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkTag(protolex, $4.start(), $4.val)
		$$ = &fieldNode{fldType: $1, name: $2, tag: $4, options: $6}
		$$.setRange($1, $8)
	}

fieldOptions : fieldOptions ',' fieldOption {
		$$ = append($1, $3...)
	}
	| fieldOption

fieldOption: optionName '=' constant {
		n := &optionNameNode{parts: $1}
		n.setRange($1[0], $1[len($1)-1])
		o := &optionNode{name: n, val: $3}
		o.setRange($1[0], $3)
		$$ = []*optionNode{o}
	}

group : _REQUIRED _GROUP name '=' _INT_LIT '{' messageBody '}' {
		checkTag(protolex, $5.start(), $5.val)
		if !unicode.IsUpper(rune($3.val[0])) {
			lexError(protolex, $3.start(), fmt.Sprintf("group %s should have a name that starts with a capital letter", $3.val))
		}
		lbl := &labelNode{basicNode: $1.toBasicNode(), required: true}
		$$ = &groupNode{groupKeyword: $2, label: lbl, name: $3, tag: $5, decls: $7}
		$$.setRange($1, $8)
	}
	| _OPTIONAL _GROUP name '=' _INT_LIT '{' messageBody '}' {
		checkTag(protolex, $5.start(), $5.val)
		if !unicode.IsUpper(rune($3.val[0])) {
			lexError(protolex, $3.start(), fmt.Sprintf("group %s should have a name that starts with a capital letter", $3.val))
		}
		lbl := &labelNode{basicNode: $1.toBasicNode()}
		$$ = &groupNode{groupKeyword: $2, label: lbl, name: $3, tag: $5, decls: $7}
		$$.setRange($1, $8)
	}
	| _REPEATED _GROUP name '=' _INT_LIT '{' messageBody '}' {
		checkTag(protolex, $5.start(), $5.val)
		if !unicode.IsUpper(rune($3.val[0])) {
			lexError(protolex, $3.start(), fmt.Sprintf("group %s should have a name that starts with a capital letter", $3.val))
		}
		lbl := &labelNode{basicNode: $1.toBasicNode(), repeated: true}
		$$ = &groupNode{groupKeyword: $2, label: lbl, name: $3, tag: $5, decls: $7}
		$$.setRange($1, $8)
	}

oneof : _ONEOF name '{' oneofBody '}' {
		c := 0
		for _, el := range $4 {
			if el.field != nil {
				c++
			}
		}
		if c == 0 {
			lexError(protolex, $1.start(), "oneof must contain at least one field")
		}
		$$ = &oneOfNode{name: $2, decls: $4}
		$$.setRange($1, $5)
	}

oneofBody : oneofBody oneofItem {
		$$ = append($1, $2...)
	}
	| oneofItem
	| {
		$$ = nil
	}

oneofItem : option {
		$$ = []*oneOfElement{{option: $1[0]}}
	}
	| oneofField {
		$$ = []*oneOfElement{{field: $1}}
	}
	| ';' {
		$$ = []*oneOfElement{{empty: $1}}
	}

oneofField : typeIdent name '=' _INT_LIT ';' {
		checkTag(protolex, $4.start(), $4.val)
		$$ = &fieldNode{fldType: $1, name: $2, tag: $4}
		$$.setRange($1, $5)
	}
	| typeIdent name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkTag(protolex, $4.start(), $4.val)
		$$ = &fieldNode{fldType: $1, name: $2, tag: $4, options: $6}
		$$.setRange($1, $8)
	}

mapField : _MAP '<' keyType ',' typeIdent '>' name '=' _INT_LIT ';' {
		checkTag(protolex, $9.start(), $9.val)
		$$ = &mapFieldNode{mapKeyword: $1, keyType: $3, valueType: $5, name: $7, tag: $9}
		$$.setRange($1, $10)
	}
	| _MAP '<' keyType ',' typeIdent '>' name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkTag(protolex, $9.start(), $9.val)
		$$ = &mapFieldNode{mapKeyword: $1, keyType: $3, valueType: $5, name: $7, tag: $9, options: $11}
		$$.setRange($1, $13)
	}

keyType : _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING

extensions : _EXTENSIONS tagRanges ';' {
		$$ = &extensionRangeNode{ranges: $2}
		$$.setRange($1, $3)
	}
	| _EXTENSIONS tagRanges '[' fieldOptions ']' ';' {
		$$ = &extensionRangeNode{ranges: $2, options: $4}
		$$.setRange($1, $6)
	}

tagRanges : tagRanges ',' tagRange {
		$$ = append($1, $3...)
	}
	| tagRange

tagRange : _INT_LIT {
		if $1.val > internal.MaxTag {
			lexError(protolex, $1.start(), fmt.Sprintf("range includes out-of-range tag: %d (should be between 0 and %d)", $1.val, internal.MaxTag))
		}
		r := &rangeNode{stNode: $1, enNode: $1, st: int32($1.val), en: int32($1.val)}
		r.setRange($1, $1)
		$$ = []*rangeNode{r}
	}
	| _INT_LIT _TO _INT_LIT {
		if $1.val > internal.MaxTag {
			lexError(protolex, $1.start(), fmt.Sprintf("range start is out-of-range tag: %d (should be between 0 and %d)", $1.val, internal.MaxTag))
		}
		if $3.val > internal.MaxTag {
			lexError(protolex, $3.start(), fmt.Sprintf("range end is out-of-range tag: %d (should be between 0 and %d)", $3.val, internal.MaxTag))
		}
		if $1.val > $3.val {
			lexError(protolex, $1.start(), fmt.Sprintf("range, %d to %d, is invalid: start must be <= end", $1.val, $3.val))
		}
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: int32($3.val)}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}
	| _INT_LIT _TO _MAX {
		if $1.val > internal.MaxTag {
			lexError(protolex, $1.start(), fmt.Sprintf("range start is out-of-range tag: %d (should be between 0 and %d)", $1.val, internal.MaxTag))
		}
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: internal.MaxTag}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}

enumRanges : enumRanges ',' enumRange {
		$$ = append($1, $3...)
	}
	| enumRange

enumRange : _INT_LIT {
		checkUint64InInt32Range(protolex, $1.start(), $1.val)
		r := &rangeNode{stNode: $1, enNode: $1, st: int32($1.val), en: int32($1.val)}
		r.setRange($1, $1)
		$$ = []*rangeNode{r}
	}
	| negIntLit {
		checkInt64InInt32Range(protolex, $1.start(), $1.val)
		r := &rangeNode{stNode: $1, enNode: $1, st: int32($1.val), en: int32($1.val)}
		r.setRange($1, $1)
		$$ = []*rangeNode{r}
	}
	| _INT_LIT _TO _INT_LIT {
		checkUint64InInt32Range(protolex, $1.start(), $1.val)
		checkUint64InInt32Range(protolex, $3.start(), $3.val)
		if $1.val > $3.val {
			lexError(protolex, $1.start(), fmt.Sprintf("range, %d to %d, is invalid: start must be <= end", $1.val, $3.val))
		}
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: int32($3.val)}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}
	| negIntLit _TO negIntLit {
		checkInt64InInt32Range(protolex, $1.start(), $1.val)
		checkInt64InInt32Range(protolex, $3.start(), $3.val)
		if $1.val > $3.val {
			lexError(protolex, $1.start(), fmt.Sprintf("range, %d to %d, is invalid: start must be <= end", $1.val, $3.val))
		}
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: int32($3.val)}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}
	| negIntLit _TO _INT_LIT {
		checkInt64InInt32Range(protolex, $1.start(), $1.val)
		checkUint64InInt32Range(protolex, $3.start(), $3.val)
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: int32($3.val)}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}
	| _INT_LIT _TO _MAX {
		checkUint64InInt32Range(protolex, $1.start(), $1.val)
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: math.MaxInt32}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}
	| negIntLit _TO _MAX {
		checkInt64InInt32Range(protolex, $1.start(), $1.val)
		r := &rangeNode{stNode: $1, enNode: $3, st: int32($1.val), en: math.MaxInt32}
		r.setRange($1, $3)
		$$ = []*rangeNode{r}
	}

msgReserved : _RESERVED tagRanges ';' {
		$$ = &reservedNode{ranges: $2}
		$$.setRange($1, $3)
	}
	| reservedNames

enumReserved : _RESERVED enumRanges ';' {
		$$ = &reservedNode{ranges: $2}
		$$.setRange($1, $3)
	}
	| reservedNames

reservedNames : _RESERVED fieldNames ';' {
		rsvd := map[string]struct{}{}
		for _, n := range $2 {
			if _, ok := rsvd[n.val]; ok {
				lexError(protolex, n.start(), fmt.Sprintf("name %q is reserved multiple times", n.val))
				break
			}
			rsvd[n.val] = struct{}{}
		}
		$$ = &reservedNode{names: $2}
		$$.setRange($1, $3)
	}

fieldNames : fieldNames ',' stringLit {
		$$ = append($1, $3)
	}
	| stringLit {
		$$ = []*stringLiteralNode{$1}
	}

enum : _ENUM name '{' enumBody '}' {
		c := 0
		for _, el := range $4 {
			if el.value != nil {
				c++
			}
		}
		if c == 0 {
			lexError(protolex, $1.start(), "enums must define at least one value")
		}
		$$ = &enumNode{name: $2, decls: $4}
		$$.setRange($1, $5)
	}

enumBody : enumBody enumItem {
		$$ = append($1, $2...)
	}
	| enumItem
	| {
		$$ = nil
	}

enumItem : option {
		$$ = []*enumElement{{option: $1[0]}}
	}
	| enumField {
		$$ = []*enumElement{{value: $1}}
	}
	| enumReserved {
		$$ = []*enumElement{{reserved: $1}}
	}
	| ';' {
		$$ = []*enumElement{{empty: $1}}
	}

enumField : name '=' _INT_LIT ';' {
		checkUint64InInt32Range(protolex, $3.start(), $3.val)
		$$ = &enumValueNode{name: $1, numberP: $3}
		$$.setRange($1, $4)
	}
	|  name '=' _INT_LIT '[' fieldOptions ']' ';' {
		checkUint64InInt32Range(protolex, $3.start(), $3.val)
		$$ = &enumValueNode{name: $1, numberP: $3, options: $5}
		$$.setRange($1, $7)
	}
	| name '=' negIntLit ';' {
		checkInt64InInt32Range(protolex, $3.start(), $3.val)
		$$ = &enumValueNode{name: $1, numberN: $3}
		$$.setRange($1, $4)
	}
	|  name '=' negIntLit '[' fieldOptions ']' ';' {
		checkInt64InInt32Range(protolex, $3.start(), $3.val)
		$$ = &enumValueNode{name: $1, numberN: $3, options: $5}
		$$.setRange($1, $7)
	}

message : _MESSAGE name '{' messageBody '}' {
		$$ = &messageNode{name: $2, decls: $4}
		$$.setRange($1, $5)
	}

messageBody : messageBody messageItem {
		$$ = append($1, $2...)
	}
	| messageItem
	| {
		$$ = nil
	}

messageItem : field {
		$$ = []*messageElement{{field: $1}}
	}
	| enum {
		$$ = []*messageElement{{enum: $1}}
	}
	| message {
		$$ = []*messageElement{{nested: $1}}
	}
	| extend {
		$$ = []*messageElement{{extend: $1}}
	}
	| extensions {
		$$ = []*messageElement{{extensionRange: $1}}
	}
	| group {
		$$ = []*messageElement{{group: $1}}
	}
	| option {
		$$ = []*messageElement{{option: $1[0]}}
	}
	| oneof {
		$$ = []*messageElement{{oneOf: $1}}
	}
	| mapField {
		$$ = []*messageElement{{mapField: $1}}
	}
	| msgReserved {
		$$ = []*messageElement{{reserved: $1}}
	}
	| ';' {
		$$ = []*messageElement{{empty: $1}}
	}

extend : _EXTEND typeIdent '{' extendBody '}' {
		c := 0
		for _, el := range $4 {
			if el.field != nil || el.group != nil {
				c++
			}
		}
		if c == 0 {
			lexError(protolex, $1.start(), "extend sections must define at least one extension")
		}
		$$ = &extendNode{extendee: $2, decls: $4}
		$$.setRange($1, $5)
	}

extendBody : extendBody extendItem {
		$$ = append($1, $2...)
	}
	| extendItem
	| {
		$$ = nil
	}

extendItem : field {
		$$ = []*extendElement{{field: $1}}
	}
	| group {
		$$ = []*extendElement{{group: $1}}
	}
	| ';' {
		$$ = []*extendElement{{empty: $1}}
	}

service : _SERVICE name '{' serviceBody '}' {
		$$ = &serviceNode{name: $2, decls: $4}
		$$.setRange($1, $5)
	}

serviceBody : serviceBody serviceItem {
		$$ = append($1, $2...)
	}
	| serviceItem
	| {
		$$ = nil
	}

// NB: doc suggests support for "stream" declaration, separate from "rpc", but
// it does not appear to be supported in protoc (doc is likely from grammar for
// Google-internal version of protoc, with support for streaming stubby)
serviceItem : option {
		$$ = []*serviceElement{{option: $1[0]}}
	}
	| rpc {
		$$ = []*serviceElement{{rpc: $1}}
	}
	| ';' {
		$$ = []*serviceElement{{empty: $1}}
	}

rpc : _RPC name '(' rpcType ')' _RETURNS '(' rpcType ')' ';' {
		$$ = &methodNode{name: $2, input: $4, output: $8}
		$$.setRange($1, $10)
	}
	| _RPC name '(' rpcType ')' _RETURNS '(' rpcType ')' '{' rpcOptions '}' {
		$$ = &methodNode{name: $2, input: $4, output: $8, options: $11}
		$$.setRange($1, $12)
	}

rpcType : _STREAM typeIdent {
		$$ = &rpcTypeNode{msgType: $2, streamKeyword: $1}
		$$.setRange($1, $2)
	}
	| typeIdent {
		$$ = &rpcTypeNode{msgType: $1}
		$$.setRange($1, $1)
	}

rpcOptions : rpcOptions rpcOption {
		$$ = append($1, $2...)
	}
	| rpcOption
	| {
		$$ = []*optionNode{}
	}

rpcOption : option {
		$$ = $1
	}
	| ';' {
		$$ = []*optionNode{}
	}

name : _NAME
	| _SYNTAX
	| _IMPORT
	| _WEAK
	| _PUBLIC
	| _PACKAGE
	| _OPTION
	| _TRUE
	| _FALSE
	| _INF
	| _NAN
	| _REPEATED
	| _OPTIONAL
	| _REQUIRED
	| _DOUBLE
	| _FLOAT
	| _INT32
	| _INT64
	| _UINT32
	| _UINT64
	| _SINT32
	| _SINT64
	| _FIXED32
	| _FIXED64
	| _SFIXED32
	| _SFIXED64
	| _BOOL
	| _STRING
	| _BYTES
	| _GROUP
	| _ONEOF
	| _MAP
	| _EXTENSIONS
	| _TO
	| _MAX
	| _RESERVED
	| _ENUM
	| _MESSAGE
	| _EXTEND
	| _SERVICE
	| _RPC
	| _STREAM
	| _RETURNS

%%
