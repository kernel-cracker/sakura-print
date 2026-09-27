// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"time"
)

// Print from any app, part 1: the IPP message format (RFC 8010): reading requests, writing answers. See docs/design/ipp.md.

// ---------- the IPP message format ----------

const (
	tagOperation = 0x01
	tagJob       = 0x02
	tagEnd       = 0x03
	tagPrinter   = 0x04
	tagUnsupport = 0x05

	vUnsupported = 0x10
	vUnknown     = 0x12
	vNoValue     = 0x13
	vInteger     = 0x21
	vBoolean     = 0x22
	vEnum        = 0x23
	vOctets      = 0x30
	vDateTime    = 0x31
	vResolution  = 0x32
	vRange       = 0x33
	vBegColl     = 0x34
	vEndColl     = 0x37
	vText        = 0x41
	vName        = 0x42
	vKeyword     = 0x44
	vURI         = 0x45
	vURIScheme   = 0x46
	vCharset     = 0x47
	vLanguage    = 0x48
	vMimeType    = 0x49
	vMemberName  = 0x4A
)

// IPP operations and status codes used here
const (
	opPrintJob          = 0x0002
	opValidateJob       = 0x0004
	opCreateJob         = 0x0005
	opSendDocument      = 0x0006
	opCancelJob         = 0x0008
	opGetJobAttributes  = 0x0009
	opGetJobs           = 0x000A
	opGetPrinterAttribs = 0x000B
	opCancelMyJobs      = 0x0039
	opCloseJob          = 0x003B
	opIdentifyPrinter   = 0x003C

	stOK                   = 0x0000
	stBadRequest           = 0x0400
	stNotFound             = 0x0406
	stNotPossible          = 0x0404
	stDocFormatUnsupported = 0x040A
	stNotAccepting         = 0x0506
	stOpUnsupported        = 0x0501
	stInternal             = 0x0500
	stVersionUnsupported   = 0x0503
	stAttrsNotSupported    = 0x040B
	stNotAuthorized        = 0x0403
)

// ippAttr is one attribute as it comes in: collections are flattened, their members named "outer.inner"
type ippAttr struct {
	group  byte
	tag    byte
	name   string
	values [][]byte
}

type ippRequest struct {
	version   [2]byte
	op        uint16
	requestID uint32
	attrs     []ippAttr
	data      io.Reader // the document, after the attributes
}

func (r *ippRequest) get(name string) *ippAttr {
	for i := range r.attrs {
		if r.attrs[i].name == name {
			return &r.attrs[i]
		}
	}
	return nil
}
func (r *ippRequest) str(name string) string {
	if a := r.get(name); a != nil && len(a.values) > 0 {
		return string(a.values[0])
	}
	return ""
}
func (r *ippRequest) num(name string) (int, bool) {
	if a := r.get(name); a != nil && len(a.values) > 0 && len(a.values[0]) == 4 {
		return int(int32(binary.BigEndian.Uint32(a.values[0]))), true
	}
	return 0, false
}

// readIPP reads the attributes; the rest of the body is the document
func readIPP(body io.Reader) (*ippRequest, error) {
	br := bufio.NewReader(body)
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, err
	}
	req := &ippRequest{version: [2]byte{hdr[0], hdr[1]}, op: binary.BigEndian.Uint16(hdr[2:]), requestID: binary.BigEndian.Uint32(hdr[4:])}
	var group byte
	var stack []string // names of the collections we're inside
	top := ""          // the last attribute outside any collection (a nameless value belongs to it)
	cur := -1          // index in req.attrs of the attribute the next nameless value belongs to
	// indexes, not pointers: req.attrs grows, and a pointer into it would go stale
	find := func(name string) int {
		for i := range req.attrs {
			if req.attrs[i].name == name {
				return i
			}
		}
		return -1
	}
	u16 := func() (int, error) {
		b := make([]byte, 2)
		_, err := io.ReadFull(br, b)
		return int(binary.BigEndian.Uint16(b)), err
	}
	for n := 0; n < 10000; n++ {
		tag, err := br.ReadByte()
		if err != nil {
			return nil, err
		}
		if tag == tagEnd {
			req.data = br
			return req, nil
		}
		if tag < 0x10 { // a new group
			group, cur, top, stack = tag, -1, "", nil
			continue
		}
		nl, err := u16()
		if err != nil {
			return nil, err
		}
		name := make([]byte, nl)
		if _, err := io.ReadFull(br, name); err != nil {
			return nil, err
		}
		vl, err := u16()
		if err != nil {
			return nil, err
		}
		if vl > 1<<16 {
			return nil, errors.New("attribute too big")
		}
		val := make([]byte, vl)
		if _, err := io.ReadFull(br, val); err != nil {
			return nil, err
		}
		switch {
		case tag == vEndColl:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			cur = -1
		case tag == vMemberName:
			// the next value inside a collection is called this; a member seen in an earlier collection of the
			// same attribute (the next paper size in a list) gets its values added to the same entry, in order
			full := strings.Join(append(append([]string{}, stack...), string(val)), ".")
			if cur = find(full); cur < 0 {
				req.attrs = append(req.attrs, ippAttr{group: group, name: full})
				cur = len(req.attrs) - 1
			}
		case tag == vBegColl:
			cname := string(name)
			switch {
			case cname != "" && len(stack) == 0: // a new attribute that is a collection
				top = cname
			case cname == "" && len(stack) == 0: // another collection value of the same attribute
				cname = top
			case cname == "" && cur >= 0: // a collection as the value of a member
				cname = req.attrs[cur].name
				if i := strings.LastIndex(cname, "."); i >= 0 {
					cname = cname[i+1:]
				}
			}
			if cname != "" {
				stack = append(stack, cname)
			}
			cur = -1
		case len(name) > 0 && len(stack) == 0: // a new attribute
			top = string(name)
			req.attrs = append(req.attrs, ippAttr{group: group, tag: tag, name: top, values: [][]byte{val}})
			cur = len(req.attrs) - 1
		case len(name) == 0 && cur >= 0: // another value of the same attribute (or of a collection member)
			if req.attrs[cur].tag == 0 {
				req.attrs[cur].tag = tag
			}
			req.attrs[cur].values = append(req.attrs[cur].values, val)
		}
	}
	return nil, errors.New("too many attributes")
}

// ippOut builds a response
type ippOut struct{ bytes.Buffer }

// head: answers speak the version they were asked in (1.1 or 2.0)
func (o *ippOut) head(version [2]byte, status uint16, id uint32) {
	o.Write(version[:])
	binary.Write(o, binary.BigEndian, status)
	binary.Write(o, binary.BigEndian, id)
}
func (o *ippOut) group(g byte) { o.WriteByte(g) }
func (o *ippOut) raw(tag byte, name string, val []byte) {
	o.WriteByte(tag)
	binary.Write(o, binary.BigEndian, uint16(len(name)))
	o.WriteString(name)
	binary.Write(o, binary.BigEndian, uint16(len(val)))
	o.Write(val)
}

// attr writes an attribute with one or more values (strings, ints, bools, or pre-encoded)
func (o *ippOut) attr(tag byte, name string, vals ...any) {
	for i, v := range vals {
		n := name
		if i > 0 {
			n = ""
		}
		o.raw(tag, n, ippValue(v))
	}
}
func ippValue(v any) []byte {
	switch x := v.(type) {
	case string:
		return []byte(x)
	case int:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(int32(x)))
		return b
	case bool:
		if x {
			return []byte{1}
		}
		return []byte{0}
	case []byte:
		return x
	case time.Time: // dateTime (RFC 2579), in UTC
		x = x.UTC()
		return []byte{byte(x.Year() >> 8), byte(x.Year()), byte(x.Month()), byte(x.Day()), byte(x.Hour()), byte(x.Minute()), byte(x.Second()), 0, '+', 0, 0}
	}
	return nil
}
func rangeOf(a, b int) []byte { return append(ippValue(a), ippValue(b)...) }
func resolution(x, y int) []byte {
	return append(append(ippValue(x), ippValue(y)...), 3) // 3 = dots per inch
}

// member writes one member of a collection; coll opens a collection value
func (o *ippOut) member(name string, tag byte, v any) {
	o.raw(vMemberName, "", []byte(name))
	o.raw(tag, "", ippValue(v))
}
func (o *ippOut) collStart(name string) { o.raw(vBegColl, name, nil) }
func (o *ippOut) collEnd()              { o.raw(vEndColl, "", nil) }
func (o *ippOut) memberColl(name string) {
	o.raw(vMemberName, "", []byte(name))
	o.raw(vBegColl, "", nil)
}
