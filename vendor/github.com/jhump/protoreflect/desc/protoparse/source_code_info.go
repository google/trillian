package protoparse

import (
	"bytes"
	"reflect"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/jhump/protoreflect/desc/internal"
)

func (r *parseResult) generateSourceCodeInfo() *dpb.SourceCodeInfo {
	if r.nodes == nil {
		// skip files that do not have AST info (these will be files
		// that came from well-known descriptors, instead of from source)
		return nil
	}

	sci := sourceCodeInfo{commentsUsed: map[*comment]struct{}{}}
	path := make([]int32, 0, 10)

	fn := r.getFileNode(r.fd).(*fileNode)
	if fn.syntax != nil {
		sci.newLoc(fn.syntax, append(path, internal.File_syntaxTag))
	}
	if fn.pkg != nil {
		sci.newLoc(fn.pkg, append(path, internal.File_packageTag))
	}
	for i, imp := range fn.imports {
		sci.newLoc(imp, append(path, internal.File_dependencyTag, int32(i)))
	}

	// file options
	r.generateSourceCodeInfoForOptions(&sci, fn.decls, func(n interface{}) *optionNode {
		return n.(*fileElement).option
	}, r.fd.Options.GetUninterpretedOption(), append(path, internal.File_optionsTag))

	// message types
	for i, msg := range r.fd.GetMessageType() {
		r.generateSourceCodeInfoForMessage(&sci, msg, append(path, internal.File_messagesTag, int32(i)))
	}

	// enum types
	for i, enum := range r.fd.GetEnumType() {
		r.generateSourceCodeInfoForEnum(&sci, enum, append(path, internal.File_enumsTag, int32(i)))
	}

	// extension fields
	for i, ext := range r.fd.GetExtension() {
		r.generateSourceCodeInfoForField(&sci, ext, append(path, internal.File_extensionsTag, int32(i)))
	}

	// services and methods
	for i, svc := range r.fd.GetService() {
		n := r.getServiceNode(svc).(*serviceNode)
		svcPath := append(path, internal.File_servicesTag, int32(i))
		sci.newLoc(n, svcPath)
		sci.newLoc(n.name, append(svcPath, internal.Service_nameTag))

		// service options
		r.generateSourceCodeInfoForOptions(&sci, n.decls, func(n interface{}) *optionNode {
			return n.(*serviceElement).option
		}, svc.Options.GetUninterpretedOption(), append(svcPath, internal.Service_optionsTag))

		// methods
		for j, mtd := range svc.GetMethod() {
			mn := r.getMethodNode(mtd).(*methodNode)
			mtdPath := append(svcPath, internal.Service_methodsTag, int32(j))
			sci.newLoc(mn, mtdPath)
			sci.newLoc(mn.name, append(mtdPath, internal.Method_nameTag))

			sci.newLoc(mn.input.msgType, append(mtdPath, internal.Method_inputTag))
			if mn.input.streamKeyword != nil {
				sci.newLoc(mn.input.streamKeyword, append(mtdPath, internal.Method_inputStreamTag))
			}
			sci.newLoc(mn.output.msgType, append(mtdPath, internal.Method_outputTag))
			if mn.output.streamKeyword != nil {
				sci.newLoc(mn.output.streamKeyword, append(mtdPath, internal.Method_outputStreamTag))
			}

			// method options
			r.generateSourceCodeInfoForOptions(&sci, mn.options, func(n interface{}) *optionNode {
				return n.(*optionNode)
			}, mtd.Options.GetUninterpretedOption(), append(mtdPath, internal.Method_optionsTag))
		}
	}
	return &dpb.SourceCodeInfo{Location: sci.generateLocs()}
}

func (r *parseResult) generateSourceCodeInfoForOptions(sci *sourceCodeInfo, elements interface{}, extractor func(interface{}) *optionNode, uninterp []*dpb.UninterpretedOption, path []int32) {
	// Known options are option node elements that have a corresponding
	// path in r.interpretedOptions. We'll do those first.
	rv := reflect.ValueOf(elements)
	for i := 0; i < rv.Len(); i++ {
		on := extractor(rv.Index(i).Interface())
		if on == nil {
			continue
		}
		optPath := r.interpretedOptions[on]
		if len(optPath) > 0 {
			p := path
			if optPath[0] == -1 {
				// used by "default" and "json_name" field pseudo-options
				// to attribute path to parent element (since those are
				// stored directly on the descriptor, not its options)
				p = make([]int32, len(path)-1)
				copy(p, path)
				optPath = optPath[1:]
			}
			sci.newLoc(on, append(p, optPath...))
		}
	}

	// Now uninterpreted options
	for i, uo := range uninterp {
		optPath := append(path, internal.UninterpretedOptionsTag, int32(i))
		on := r.getOptionNode(uo).(*optionNode)
		sci.newLoc(on, optPath)

		var valTag int32
		switch {
		case uo.IdentifierValue != nil:
			valTag = internal.Uninterpreted_identTag
		case uo.PositiveIntValue != nil:
			valTag = internal.Uninterpreted_posIntTag
		case uo.NegativeIntValue != nil:
			valTag = internal.Uninterpreted_negIntTag
		case uo.DoubleValue != nil:
			valTag = internal.Uninterpreted_doubleTag
		case uo.StringValue != nil:
			valTag = internal.Uninterpreted_stringTag
		case uo.AggregateValue != nil:
			valTag = internal.Uninterpreted_aggregateTag
		}
		if valTag != 0 {
			sci.newLoc(on.val, append(optPath, valTag))
		}

		for j, n := range uo.Name {
			optNmPath := append(optPath, internal.Uninterpreted_nameTag, int32(j))
			nn := r.getOptionNamePartNode(n).(*optionNamePartNode)
			sci.newLoc(nn, optNmPath)
			sci.newLoc(nn.text, append(optNmPath, internal.UninterpretedName_nameTag))
		}
	}
}

func (r *parseResult) generateSourceCodeInfoForMessage(sci *sourceCodeInfo, msg *dpb.DescriptorProto, path []int32) {
	n := r.getMessageNode(msg)
	sci.newLoc(n, path)

	var decls []*messageElement
	var resvdNames []*stringLiteralNode
	switch n := n.(type) {
	case *messageNode:
		decls = n.decls
		resvdNames = n.reserved
	case *groupNode:
		decls = n.decls
		resvdNames = n.reserved
	}
	if decls == nil {
		// map entry so nothing else to do
		return
	}

	sci.newLoc(n.messageName(), append(path, internal.Message_nameTag))

	// message options
	r.generateSourceCodeInfoForOptions(sci, decls, func(n interface{}) *optionNode {
		return n.(*messageElement).option
	}, msg.Options.GetUninterpretedOption(), append(path, internal.Message_optionsTag))

	// fields
	for i, fld := range msg.GetField() {
		r.generateSourceCodeInfoForField(sci, fld, append(path, internal.Message_fieldsTag, int32(i)))
	}

	// one-ofs
	for i, ood := range msg.GetOneofDecl() {
		oon := r.getOneOfNode(ood).(*oneOfNode)
		ooPath := append(path, internal.Message_oneOfsTag, int32(i))
		sci.newLoc(oon, ooPath)
		sci.newLoc(oon.name, append(ooPath, internal.OneOf_nameTag))

		// one-of options
		r.generateSourceCodeInfoForOptions(sci, oon.decls, func(n interface{}) *optionNode {
			return n.(*oneOfElement).option
		}, ood.Options.GetUninterpretedOption(), append(ooPath, internal.OneOf_optionsTag))
	}

	// nested messages
	for i, nm := range msg.GetNestedType() {
		r.generateSourceCodeInfoForMessage(sci, nm, append(path, internal.Message_nestedMessagesTag, int32(i)))
	}

	// nested enums
	for i, enum := range msg.GetEnumType() {
		r.generateSourceCodeInfoForEnum(sci, enum, append(path, internal.Message_enumsTag, int32(i)))
	}

	// nested extensions
	for i, ext := range msg.GetExtension() {
		r.generateSourceCodeInfoForField(sci, ext, append(path, internal.Message_extensionsTag, int32(i)))
	}

	// extension ranges
	for i, er := range msg.ExtensionRange {
		rangePath := append(path, internal.Message_extensionRangeTag, int32(i))
		rn := r.getExtensionRangeNode(er).(*rangeNode)
		sci.newLoc(rn, rangePath)
		sci.newLoc(rn.stNode, append(rangePath, internal.ExtensionRange_startTag))
		if rn.stNode != rn.enNode {
			sci.newLoc(rn.enNode, append(rangePath, internal.ExtensionRange_endTag))
		}
		// now we have to find the extension decl and options that correspond to this range :(
		for _, d := range decls {
			found := false
			if d.extensionRange != nil {
				for _, r := range d.extensionRange.ranges {
					if rn == r {
						found = true
						break
					}
				}
			}
			if found {
				r.generateSourceCodeInfoForOptions(sci, d.extensionRange.options, func(n interface{}) *optionNode {
					return n.(*optionNode)
				}, er.Options.GetUninterpretedOption(), append(rangePath, internal.ExtensionRange_optionsTag))
				break
			}
		}
	}

	// reserved ranges
	for i, rr := range msg.ReservedRange {
		rangePath := append(path, internal.Message_reservedRangeTag, int32(i))
		rn := r.getMessageReservedRangeNode(rr).(*rangeNode)
		sci.newLoc(rn, rangePath)
		sci.newLoc(rn.stNode, append(rangePath, internal.ReservedRange_startTag))
		if rn.stNode != rn.enNode {
			sci.newLoc(rn.enNode, append(rangePath, internal.ReservedRange_endTag))
		}
	}

	// reserved names
	for i, n := range resvdNames {
		sci.newLoc(n, append(path, internal.Message_reservedNameTag, int32(i)))
	}
}

func (r *parseResult) generateSourceCodeInfoForEnum(sci *sourceCodeInfo, enum *dpb.EnumDescriptorProto, path []int32) {
	n := r.getEnumNode(enum).(*enumNode)
	sci.newLoc(n, path)
	sci.newLoc(n.name, append(path, internal.Enum_nameTag))

	// enum options
	r.generateSourceCodeInfoForOptions(sci, n.decls, func(n interface{}) *optionNode {
		return n.(*enumElement).option
	}, enum.Options.GetUninterpretedOption(), append(path, internal.Enum_optionsTag))

	// enum values
	for j, ev := range enum.GetValue() {
		evn := r.getEnumValueNode(ev).(*enumValueNode)
		evPath := append(path, internal.Enum_valuesTag, int32(j))
		sci.newLoc(evn, evPath)
		sci.newLoc(evn.name, append(evPath, internal.EnumVal_nameTag))
		sci.newLoc(evn.getNumber(), append(evPath, internal.EnumVal_numberTag))

		// enum value options
		r.generateSourceCodeInfoForOptions(sci, evn.options, func(n interface{}) *optionNode {
			return n.(*optionNode)
		}, ev.Options.GetUninterpretedOption(), append(evPath, internal.EnumVal_optionsTag))
	}

	// reserved ranges
	for i, rr := range enum.GetReservedRange() {
		rangePath := append(path, internal.Enum_reservedRangeTag, int32(i))
		rn := r.getEnumReservedRangeNode(rr).(*rangeNode)
		sci.newLoc(rn, rangePath)
		sci.newLoc(rn.stNode, append(rangePath, internal.ReservedRange_startTag))
		if rn.stNode != rn.enNode {
			sci.newLoc(rn.enNode, append(rangePath, internal.ReservedRange_endTag))
		}
	}

	// reserved names
	for i, rn := range n.reserved {
		sci.newLoc(rn, append(path, internal.Enum_reservedNameTag, int32(i)))
	}
}

func (r *parseResult) generateSourceCodeInfoForField(sci *sourceCodeInfo, fld *dpb.FieldDescriptorProto, path []int32) {
	n := r.getFieldNode(fld)

	isGroup := false
	var opts []*optionNode
	var extendee *extendNode
	switch n := n.(type) {
	case *fieldNode:
		opts = n.options
		extendee = n.extendee
	case *mapFieldNode:
		opts = n.options
	case *groupNode:
		isGroup = true
		extendee = n.extendee
	case *syntheticMapField:
		// shouldn't get here since we don't recurse into fields from a mapNode
		// in generateSourceCodeInfoForMessage... but just in case
		return
	}

	sci.newLoc(n, path)
	if !isGroup {
		sci.newLoc(n.fieldName(), append(path, internal.Field_nameTag))
		sci.newLoc(n.fieldType(), append(path, internal.Field_typeTag))
	}
	if n.fieldLabel() != nil {
		sci.newLoc(n.fieldLabel(), append(path, internal.Field_labelTag))
	}
	sci.newLoc(n.fieldTag(), append(path, internal.Field_numberTag))
	if extendee != nil {
		sci.newLoc(extendee.extendee, append(path, internal.Field_extendeeTag))
	}

	r.generateSourceCodeInfoForOptions(sci, opts, func(n interface{}) *optionNode {
		return n.(*optionNode)
	}, fld.Options.GetUninterpretedOption(), append(path, internal.Field_optionsTag))
}

type sourceCodeInfo struct {
	locs         []*dpb.SourceCodeInfo_Location
	commentsUsed map[*comment]struct{}
}

func (sci *sourceCodeInfo) newLoc(n node, path []int32) {
	leadingComments := n.leadingComments()
	trailingComments := n.trailingComments()
	if sci.commentUsed(leadingComments) {
		leadingComments = nil
	}
	if sci.commentUsed(trailingComments) {
		trailingComments = nil
	}
	detached := groupComments(leadingComments)
	trail := combineComments(trailingComments)
	var lead *string
	if len(leadingComments) > 0 && leadingComments[len(leadingComments)-1].end.Line >= n.start().Line-1 {
		lead = proto.String(detached[len(detached)-1])
		detached = detached[:len(detached)-1]
	}
	dup := make([]int32, len(path))
	copy(dup, path)
	var span []int32
	if n.start().Line == n.end().Line {
		span = []int32{int32(n.start().Line) - 1, int32(n.start().Col) - 1, int32(n.end().Col) - 1}
	} else {
		span = []int32{int32(n.start().Line) - 1, int32(n.start().Col) - 1, int32(n.end().Line) - 1, int32(n.end().Col) - 1}
	}
	sci.locs = append(sci.locs, &dpb.SourceCodeInfo_Location{
		LeadingDetachedComments: detached,
		LeadingComments:         lead,
		TrailingComments:        trail,
		Path:                    dup,
		Span:                    span,
	})
}

func (sci *sourceCodeInfo) commentUsed(c []*comment) bool {
	if len(c) == 0 {
		return false
	}
	if _, ok := sci.commentsUsed[c[0]]; ok {
		return true
	}

	sci.commentsUsed[c[0]] = struct{}{}
	return false
}

func groupComments(comments []*comment) []string {
	if len(comments) == 0 {
		return nil
	}

	var groups []string
	singleLineStyle := comments[0].text[:2] == "//"
	line := comments[0].end.Line
	start := 0
	for i := 1; i < len(comments); i++ {
		c := comments[i]
		prevSingleLine := singleLineStyle
		singleLineStyle = strings.HasPrefix(comments[i].text, "//")
		if !singleLineStyle || prevSingleLine != singleLineStyle || c.start.Line > line+1 {
			// new group!
			groups = append(groups, *combineComments(comments[start:i]))
			start = i
		}
		line = c.end.Line
	}
	// don't forget last group
	groups = append(groups, *combineComments(comments[start:]))

	return groups
}

func combineComments(comments []*comment) *string {
	if len(comments) == 0 {
		return nil
	}
	first := true
	var buf bytes.Buffer
	for _, c := range comments {
		if first {
			first = false
		} else {
			buf.WriteByte('\n')
		}
		if c.text[:2] == "//" {
			buf.WriteString(c.text[2:])
		} else {
			lines := strings.Split(c.text[2:len(c.text)-2], "\n")
			first := true
			for _, l := range lines {
				if first {
					first = false
				} else {
					buf.WriteByte('\n')
				}

				// strip a prefix of whitespace followed by '*'
				j := 0
				for j < len(l) {
					if l[j] != ' ' && l[j] != '\t' {
						break
					}
					j++
				}
				if j == len(l) {
					l = ""
				} else if l[j] == '*' {
					l = l[j+1:]
				} else if j > 0 {
					l = " " + l[j:]
				}

				buf.WriteString(l)
			}
		}
	}
	return proto.String(buf.String())
}

func (sci *sourceCodeInfo) generateLocs() []*dpb.SourceCodeInfo_Location {
	// generate intermediate locations: paths between root (inclusive) and the
	// leaf locations already created, these will not have comments but will
	// have aggregate span, than runs from min(start pos) to max(end pos) for
	// all descendent paths.

	if len(sci.locs) == 0 {
		// nothing to generate
		return nil
	}

	var root locTrie
	for _, loc := range sci.locs {
		root.add(loc.Path, loc)
	}
	root.fillIn()
	locs := make([]*dpb.SourceCodeInfo_Location, 0, root.countLocs())
	root.aggregate(&locs)
	// finally, sort the resulting slice by location
	sort.Slice(locs, func(i, j int) bool {
		startI, endI := getSpanPositions(locs[i].Span)
		startJ, endJ := getSpanPositions(locs[j].Span)
		cmp := compareSlice(startI, startJ)
		if cmp == 0 {
			// if start position is the same, sort by end position _decreasing_
			// (so enclosing locations will appear before leaves)
			cmp = -compareSlice(endI, endJ)
			if cmp == 0 {
				// start and end position are the same? so break ties using path
				cmp = compareSlice(locs[i].Path, locs[j].Path)
			}
		}
		return cmp < 0
	})
	return locs
}

type locTrie struct {
	children map[int32]*locTrie
	loc      *dpb.SourceCodeInfo_Location
}

func (t *locTrie) add(path []int32, loc *dpb.SourceCodeInfo_Location) {
	if len(path) == 0 {
		t.loc = loc
		return
	}
	child := t.children[path[0]]
	if child == nil {
		if t.children == nil {
			t.children = map[int32]*locTrie{}
		}
		child = &locTrie{}
		t.children[path[0]] = child
	}
	child.add(path[1:], loc)
}

func (t *locTrie) fillIn() {
	var path []int32
	var start, end []int32
	for _, child := range t.children {
		// recurse
		child.fillIn()
		if t.loc == nil {
			// maintain min(start) and max(end) so we can
			// populate t.loc below
			childStart, childEnd := getSpanPositions(child.loc.Span)

			if start == nil {
				if path == nil {
					path = child.loc.Path[:len(child.loc.Path)-1]
				}
				start = childStart
				end = childEnd
			} else {
				if compareSlice(childStart, start) < 0 {
					start = childStart
				}
				if compareSlice(childEnd, end) > 0 {
					end = childEnd
				}
			}
		}
	}

	if t.loc == nil {
		var span []int32
		// we don't use append below because we want a new slice
		// that doesn't share underlying buffer with spans from
		// any other location
		if start[0] == end[0] {
			span = []int32{start[0], start[1], end[1]}
		} else {
			span = []int32{start[0], start[1], end[0], end[1]}
		}
		t.loc = &dpb.SourceCodeInfo_Location{
			Path: path,
			Span: span,
		}
	}
}

func (t *locTrie) countLocs() int {
	count := 0
	if t.loc != nil {
		count = 1
	}
	for _, ch := range t.children {
		count += ch.countLocs()
	}
	return count
}

func (t *locTrie) aggregate(dest *[]*dpb.SourceCodeInfo_Location) {
	if t.loc != nil {
		*dest = append(*dest, t.loc)
	}
	for _, child := range t.children {
		child.aggregate(dest)
	}
}

func getSpanPositions(span []int32) (start, end []int32) {
	start = span[:2]
	if len(span) == 3 {
		end = []int32{span[0], span[2]}
	} else {
		end = span[2:]
	}
	return
}

func compareSlice(a, b []int32) int {
	end := len(a)
	if len(b) < end {
		end = len(b)
	}
	for i := 0; i < end; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
