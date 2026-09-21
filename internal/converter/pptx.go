package converter

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// emuPerPixel converts CSS pixels to the English Metric Units OOXML measures
// in: 914400 EMU per inch at 96 px per inch.
const emuPerPixel = 914400 / cssPixelsPerInch

// pptxDeck is everything a PowerPoint file is written from: one pre-rendered
// image per slide, laid over the whole slide, plus its presenter notes.
type pptxDeck struct {
	size   slideSize
	title  string
	slides []pptxSlide
}

// pptxSlide is one slide: its rendered PNG and its presenter notes, empty when
// it has none.
type pptxSlide struct {
	png   []byte
	notes string
}

// Namespaces and relationship types used across the package.
const (
	nsA     = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsR     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsP     = "http://schemas.openxmlformats.org/presentationml/2006/main"
	relDoc  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	ctPML   = "application/vnd.openxmlformats-officedocument.presentationml."
	xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
)

// writePPTX writes deck as an Office Open XML presentation at path.
//
// The package is the smallest one PowerPoint, Keynote, Google Slides and
// LibreOffice all open: one slide master and one blank layout, a theme (a
// master cannot exist without one), and per slide a single picture filling it.
// Notes parts — a notes master with its own theme, and a notes slide per slide
// that has notes — are written only when some slide has notes, because a
// presentation that references a notes master it does not contain is invalid.
func writePPTX(path string, deck pptxDeck) error {
	if len(deck.slides) == 0 {
		return errors.New("a presentation needs at least one slide")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w := &pptxWriter{zw: zw, deck: deck}
	w.writeAll()
	if w.err != nil {
		return w.err
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish pptx: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil { //nolint:gosec // G306: a document the caller asked to be written
		return fmt.Errorf("write pptx: %w", err)
	}
	return nil
}

// pptxWriter accumulates the parts of one package, keeping the first error so
// the part writers can be called in sequence without checking each one.
type pptxWriter struct {
	zw   *zip.Writer
	deck pptxDeck
	err  error
}

func (w *pptxWriter) part(name, content string) {
	w.bytesPart(name, []byte(content))
}

func (w *pptxWriter) bytesPart(name string, content []byte) {
	if w.err != nil {
		return
	}
	f, err := w.zw.Create(name)
	if err != nil {
		w.err = fmt.Errorf("add %s to pptx: %w", name, err)
		return
	}
	if _, err := f.Write(content); err != nil {
		w.err = fmt.Errorf("write %s to pptx: %w", name, err)
	}
}

func (w *pptxWriter) hasNotes() bool {
	for _, s := range w.deck.slides {
		if strings.TrimSpace(s.notes) != "" {
			return true
		}
	}
	return false
}

func (w *pptxWriter) writeAll() {
	cx := w.deck.size.widthPx * emuPerPixel
	cy := w.deck.size.heightPx * emuPerPixel
	notes := w.hasNotes()

	w.part("[Content_Types].xml", w.contentTypes(notes))
	w.part("_rels/.rels", rels(
		rel{"rId1", "officeDocument", "ppt/presentation.xml"},
		rel{"rId2", "package/2006/relationships/metadata/core-properties", "docProps/core.xml"},
		rel{"rId3", "extended-properties", "docProps/app.xml"},
	))
	w.part("docProps/core.xml", coreProps(w.deck.title))
	w.part("docProps/app.xml", xmlDecl+`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">`+
		`<Application>md2pdf</Application>`+fmt.Sprintf("<Slides>%d</Slides>", len(w.deck.slides))+`</Properties>`)

	// presentation.xml and its relationships.
	presRels := []rel{{"rId1", "slideMaster", "slideMasters/slideMaster1.xml"}}
	var sldIDs strings.Builder
	for i := range w.deck.slides {
		id := fmt.Sprintf("rId%d", i+2)
		presRels = append(presRels, rel{id, "slide", fmt.Sprintf("slides/slide%d.xml", i+1)})
		fmt.Fprintf(&sldIDs, `<p:sldId id="%d" r:id="%s"/>`, 256+i, id)
	}
	next := len(w.deck.slides) + 2
	notesMasterList := ""
	if notes {
		id := fmt.Sprintf("rId%d", next)
		next++
		presRels = append(presRels, rel{id, "notesMaster", "notesMasters/notesMaster1.xml"})
		notesMasterList = `<p:notesMasterIdLst><p:notesMasterId r:id="` + id + `"/></p:notesMasterIdLst>`
	}
	presRels = append(presRels,
		rel{fmt.Sprintf("rId%d", next), "theme", "theme/theme1.xml"},
		rel{fmt.Sprintf("rId%d", next+1), "presProps", "presProps.xml"},
		rel{fmt.Sprintf("rId%d", next+2), "viewProps", "viewProps.xml"},
		rel{fmt.Sprintf("rId%d", next+3), "tableStyles", "tableStyles.xml"},
	)
	w.part("ppt/presentation.xml", xmlDecl+`<p:presentation xmlns:a="`+nsA+`" xmlns:r="`+nsR+`" xmlns:p="`+nsP+`" saveSubsetFonts="1">`+
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`+
		notesMasterList+
		`<p:sldIdLst>`+sldIDs.String()+`</p:sldIdLst>`+
		fmt.Sprintf(`<p:sldSz cx="%d" cy="%d"/>`, cx, cy)+
		`<p:notesSz cx="6858000" cy="9144000"/>`+
		`</p:presentation>`)
	w.part("ppt/_rels/presentation.xml.rels", rels(presRels...))
	w.part("ppt/presProps.xml", xmlDecl+`<p:presentationPr xmlns:a="`+nsA+`" xmlns:r="`+nsR+`" xmlns:p="`+nsP+`"/>`)
	w.part("ppt/viewProps.xml", xmlDecl+`<p:viewPr xmlns:a="`+nsA+`" xmlns:r="`+nsR+`" xmlns:p="`+nsP+`">`+
		`<p:normalViewPr><p:restoredLeft sz="15620"/><p:restoredTop sz="94660"/></p:normalViewPr>`+
		`<p:gridSpacing cx="76200" cy="76200"/></p:viewPr>`)
	w.part("ppt/tableStyles.xml", xmlDecl+`<a:tblStyleLst xmlns:a="`+nsA+`" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`)

	w.part("ppt/theme/theme1.xml", themeXML("md2pdf"))
	w.part("ppt/slideMasters/slideMaster1.xml", xmlDecl+`<p:sldMaster xmlns:a="`+nsA+`" xmlns:r="`+nsR+`" xmlns:p="`+nsP+`">`+
		`<p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg>`+emptySpTree+`</p:cSld>`+
		clrMap+
		`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>`+
		`<p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles>`+
		`</p:sldMaster>`)
	w.part("ppt/slideMasters/_rels/slideMaster1.xml.rels", rels(
		rel{"rId1", "slideLayout", "../slideLayouts/slideLayout1.xml"},
		rel{"rId2", "theme", "../theme/theme1.xml"},
	))
	w.part("ppt/slideLayouts/slideLayout1.xml", xmlDecl+`<p:sldLayout xmlns:a="`+nsA+`" xmlns:r="`+nsR+`" xmlns:p="`+nsP+`" type="blank" preserve="1">`+
		`<p:cSld name="Blank">`+emptySpTree+`</p:cSld>`+
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`)
	w.part("ppt/slideLayouts/_rels/slideLayout1.xml.rels", rels(
		rel{"rId1", "slideMaster", "../slideMasters/slideMaster1.xml"},
	))

	for i, s := range w.deck.slides {
		n := i + 1
		w.bytesPart(fmt.Sprintf("ppt/media/image%d.png", n), s.png)
		w.part(fmt.Sprintf("ppt/slides/slide%d.xml", n), slideXML(n, cx, cy))
		slideRels := []rel{
			{"rId1", "slideLayout", "../slideLayouts/slideLayout1.xml"},
			{"rId2", "image", fmt.Sprintf("../media/image%d.png", n)},
		}
		if strings.TrimSpace(s.notes) != "" {
			slideRels = append(slideRels, rel{"rId3", "notesSlide", fmt.Sprintf("../notesSlides/notesSlide%d.xml", n)})
			w.part(fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", n), notesSlideXML(s.notes))
			w.part(fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", n), rels(
				rel{"rId1", "notesMaster", "../notesMasters/notesMaster1.xml"},
				rel{"rId2", "slide", fmt.Sprintf("../slides/slide%d.xml", n)},
			))
		}
		w.part(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n), rels(slideRels...))
	}

	if notes {
		// A notes master needs a theme of its own; sharing the slide master's
		// is not something every consumer accepts.
		w.part("ppt/theme/theme2.xml", themeXML("md2pdf notes"))
		w.part("ppt/notesMasters/notesMaster1.xml", xmlDecl+`<p:notesMaster xmlns:a="`+nsA+`" xmlns:r="`+nsR+`" xmlns:p="`+nsP+`">`+
			`<p:cSld>`+emptySpTree+`</p:cSld>`+clrMap+`</p:notesMaster>`)
		w.part("ppt/notesMasters/_rels/notesMaster1.xml.rels", rels(
			rel{"rId1", "theme", "../theme/theme2.xml"},
		))
	}
}

func (w *pptxWriter) contentTypes(notes bool) string {
	var sb strings.Builder
	sb.WriteString(xmlDecl + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	sb.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	sb.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	sb.WriteString(`<Default Extension="png" ContentType="image/png"/>`)
	override := func(part, ct string) {
		fmt.Fprintf(&sb, `<Override PartName="/%s" ContentType="%s"/>`, part, ct)
	}
	override("ppt/presentation.xml", ctPML+"presentation.main+xml")
	override("ppt/presProps.xml", ctPML+"presProps+xml")
	override("ppt/viewProps.xml", ctPML+"viewProps+xml")
	override("ppt/tableStyles.xml", ctPML+"tableStyles+xml")
	override("ppt/slideMasters/slideMaster1.xml", ctPML+"slideMaster+xml")
	override("ppt/slideLayouts/slideLayout1.xml", ctPML+"slideLayout+xml")
	override("ppt/theme/theme1.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	override("docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml")
	override("docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml")
	for i, s := range w.deck.slides {
		override(fmt.Sprintf("ppt/slides/slide%d.xml", i+1), ctPML+"slide+xml")
		if strings.TrimSpace(s.notes) != "" {
			override(fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i+1), ctPML+"notesSlide+xml")
		}
	}
	if notes {
		override("ppt/notesMasters/notesMaster1.xml", ctPML+"notesMaster+xml")
		override("ppt/theme/theme2.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	}
	sb.WriteString(`</Types>`)
	return sb.String()
}

// rel is one relationship: its id, its type (the tail after the officeDocument
// relationships namespace, or a full package path starting with "package/"),
// and its target relative to the source part's directory.
type rel struct {
	id, kind, target string
}

func rels(list ...rel) string {
	var sb strings.Builder
	sb.WriteString(xmlDecl + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range list {
		kind := relDoc + r.kind
		if strings.HasPrefix(r.kind, "package/") {
			kind = "http://schemas.openxmlformats.org/" + r.kind
		}
		fmt.Fprintf(&sb, `<Relationship Id="%s" Type="%s" Target="%s"/>`, r.id, kind, r.target)
	}
	sb.WriteString(`</Relationships>`)
	return sb.String()
}

// emptySpTree is the shape tree every slide-like part must have, with only its
// mandatory group properties.
const emptySpTree = `<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree>`

// clrMap maps the theme's colors onto the roles masters refer to.
const clrMap = `<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" ` +
	`accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`

// slideXML is slide n: one picture covering the whole slide.
func slideXML(n, cx, cy int) string {
	return xmlDecl + `<p:sld xmlns:a="` + nsA + `" xmlns:r="` + nsR + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
		fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="2" name="Slide %d"/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`, n) +
		`<p:blipFill><a:blip r:embed="rId2"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>` +
		fmt.Sprintf(`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`, cx, cy) +
		`</p:pic></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
}

// notesSlideXML is a notes page: the slide thumbnail placeholder on top and
// the notes text below, one paragraph per line. Both placeholders carry their
// own position, since the notes master defines none to inherit.
func notesSlideXML(notes string) string {
	var paras strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(notes), "\n") {
		if line == "" {
			paras.WriteString(`<a:p><a:endParaRPr lang="ja-JP"/></a:p>`)
			continue
		}
		paras.WriteString(`<a:p><a:r><a:rPr lang="ja-JP"/><a:t>` + xmlEscape(line) + `</a:t></a:r></a:p>`)
	}
	return xmlDecl + `<p:notes xmlns:a="` + nsA + `" xmlns:r="` + nsR + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr>` +
		`<p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="685800" y="1143000"/><a:ext cx="5486400" cy="3086100"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>` +
		`<p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="685800" y="4400550"/><a:ext cx="5486400" cy="3600450"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + paras.String() + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`
}

// coreProps is docProps/core.xml, carrying the deck's title.
func coreProps(title string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	return xmlDecl + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" ` +
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + xmlEscape(title) + `</dc:title><dc:creator>md2pdf</dc:creator>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created>` +
		`<dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

// xmlEscape escapes text for an XML element or attribute.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s)) // writing to a bytes.Buffer cannot fail
	return buf.String()
}

// themeXML is a minimal but complete theme: the color, font and format
// schemes a theme must declare, each with the entries the schema requires.
// Slides are pictures, so none of it is visible; it exists because a master
// without a theme is not a valid presentation.
func themeXML(name string) string {
	solid := func(c string) string { return `<a:solidFill><a:schemeClr val="` + c + `"/></a:solidFill>` }
	line := `<a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>`
	effect := `<a:effectStyle><a:effectLst/></a:effectStyle>`
	return xmlDecl + `<a:theme xmlns:a="` + nsA + `" name="` + xmlEscape(name) + `"><a:themeElements>` +
		`<a:clrScheme name="md2pdf">` +
		`<a:dk1><a:srgbClr val="000000"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="1F2328"/></a:dk2><a:lt2><a:srgbClr val="F6F8FA"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="0969DA"/></a:accent1><a:accent2><a:srgbClr val="1A7F37"/></a:accent2>` +
		`<a:accent3><a:srgbClr val="9A6700"/></a:accent3><a:accent4><a:srgbClr val="CF222E"/></a:accent4>` +
		`<a:accent5><a:srgbClr val="8250DF"/></a:accent5><a:accent6><a:srgbClr val="BF3989"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="0969DA"/></a:hlink><a:folHlink><a:srgbClr val="8250DF"/></a:folHlink>` +
		`</a:clrScheme>` +
		`<a:fontScheme name="md2pdf">` +
		`<a:majorFont><a:latin typeface="Arial"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont>` +
		`<a:minorFont><a:latin typeface="Arial"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont>` +
		`</a:fontScheme>` +
		`<a:fmtScheme name="md2pdf">` +
		`<a:fillStyleLst>` + solid("phClr") + solid("phClr") + solid("phClr") + `</a:fillStyleLst>` +
		`<a:lnStyleLst>` + line + line + line + `</a:lnStyleLst>` +
		`<a:effectStyleLst>` + effect + effect + effect + `</a:effectStyleLst>` +
		`<a:bgFillStyleLst>` + solid("phClr") + solid("phClr") + solid("phClr") + `</a:bgFillStyleLst>` +
		`</a:fmtScheme>` +
		`</a:themeElements></a:theme>`
}
