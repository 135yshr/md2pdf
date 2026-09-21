package converter

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeSlidePNG writes a PNG of the given size to a temporary file and returns
// its path, which is how captured slides reach writePPTX.
func fakeSlidePNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	path := filepath.Join(t.TempDir(), "slide.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write png: %v", err)
	}
	return path
}

// readPPTX opens a written .pptx and returns every part by name.
func readPPTX(t *testing.T, file string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(file)
	if err != nil {
		t.Fatalf("open pptx as zip: %v", err)
	}
	defer zr.Close()
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		parts[f.Name] = data
	}
	return parts
}

func writeTestDeck(t *testing.T, deck pptxDeck) map[string][]byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "deck.pptx")
	if err := writePPTX(out, deck); err != nil {
		t.Fatalf("writePPTX: %v", err)
	}
	return readPPTX(t, out)
}

func threeSlideDeck(t *testing.T) pptxDeck {
	t.Helper()
	img := fakeSlidePNG(t, 64, 36)
	return pptxDeck{
		size:  slideSizes[0],
		title: "Deck <&> title",
		slides: []pptxSlide{
			{pngPath: img},
			{pngPath: img, notes: "Mention the benchmark\n\nand <escape> & this"},
			{pngPath: img},
		},
	}
}

var sldIDRE = regexp.MustCompile(`<p:sldId id="\d+" r:id="[^"]+"/>`)

func TestWritePPTX_DeclaresEverySlideAndTheSize(t *testing.T) {
	parts := writeTestDeck(t, threeSlideDeck(t))
	pres := string(parts["ppt/presentation.xml"])
	if got := len(sldIDRE.FindAllString(pres, -1)); got != 3 {
		t.Errorf("presentation.xml lists %d slides, want 3", got)
	}
	if !strings.Contains(pres, `<p:sldSz cx="12192000" cy="6858000"/>`) {
		t.Errorf("slide size is not 16:9 in EMU:\n%s", pres)
	}
	for i := 1; i <= 3; i++ {
		for _, name := range []string{
			"ppt/slides/slide%d.xml", "ppt/slides/_rels/slide%d.xml.rels", "ppt/media/image%d.png",
		} {
			part := strings.Replace(name, "%d", string(rune('0'+i)), 1)
			if _, ok := parts[part]; !ok {
				t.Errorf("missing part %s", part)
			}
		}
	}
}

func TestWritePPTX_FourByThree(t *testing.T) {
	deck := threeSlideDeck(t)
	deck.size = slideSizes[1]
	parts := writeTestDeck(t, deck)
	if !strings.Contains(string(parts["ppt/presentation.xml"]), `<p:sldSz cx="9144000" cy="6858000"/>`) {
		t.Error("slide size is not 4:3 in EMU")
	}
}

func TestWritePPTX_ImageFillsTheSlide(t *testing.T) {
	parts := writeTestDeck(t, threeSlideDeck(t))
	slide := string(parts["ppt/slides/slide1.xml"])
	for _, want := range []string{`<a:off x="0" y="0"/>`, `<a:ext cx="12192000" cy="6858000"/>`, `r:embed="`} {
		if !strings.Contains(slide, want) {
			t.Errorf("slide1.xml lacks %q", want)
		}
	}
}

func TestWritePPTX_NotesOnlyWhereThereAreNotes(t *testing.T) {
	parts := writeTestDeck(t, threeSlideDeck(t))
	notes, ok := parts["ppt/notesSlides/notesSlide2.xml"]
	if !ok {
		t.Fatal("slide 2 has notes but no notes slide")
	}
	text := string(notes)
	for _, want := range []string{"Mention the benchmark", "and &lt;escape&gt; &amp; this"} {
		if !strings.Contains(text, want) {
			t.Errorf("notesSlide2.xml lacks %q:\n%s", want, text)
		}
	}
	for _, name := range []string{"ppt/notesSlides/notesSlide1.xml", "ppt/notesSlides/notesSlide3.xml"} {
		if _, ok := parts[name]; ok {
			t.Errorf("%s exists for a slide without notes", name)
		}
	}
	if !strings.Contains(string(parts["ppt/slides/_rels/slide2.xml.rels"]), "notesSlide2.xml") {
		t.Error("slide 2 does not link its notes slide")
	}
	if _, ok := parts["ppt/notesMasters/notesMaster1.xml"]; !ok {
		t.Error("a deck with notes has no notes master")
	}
}

func TestWritePPTX_NoNotesMeansNoNotesParts(t *testing.T) {
	deck := threeSlideDeck(t)
	deck.slides[1].notes = ""
	parts := writeTestDeck(t, deck)
	for name := range parts {
		if strings.HasPrefix(name, "ppt/notes") {
			t.Errorf("unexpected notes part %s", name)
		}
	}
	if strings.Contains(string(parts["ppt/presentation.xml"]), "notesMasterIdLst") {
		t.Error("presentation.xml references a notes master that does not exist")
	}
}

func TestWritePPTX_TitleInCoreProperties(t *testing.T) {
	parts := writeTestDeck(t, threeSlideDeck(t))
	if !strings.Contains(string(parts["docProps/core.xml"]), "<dc:title>Deck &lt;&amp;&gt; title</dc:title>") {
		t.Errorf("core.xml title missing or unescaped:\n%s", parts["docProps/core.xml"])
	}
}

// TestWritePPTX_PackageIsConsistent checks what every OOXML consumer relies on:
// every XML part is well-formed, every part has a content type, and every
// internal relationship points at a part that exists.
func TestWritePPTX_PackageIsConsistent(t *testing.T) {
	for name, deck := range map[string]pptxDeck{
		"with notes":    threeSlideDeck(t),
		"without notes": {size: slideSizes[0], slides: []pptxSlide{{pngPath: fakeSlidePNG(t, 4, 4)}}},
	} {
		t.Run(name, func(t *testing.T) {
			parts := writeTestDeck(t, deck)
			for part, data := range parts {
				if strings.HasSuffix(part, ".xml") || strings.HasSuffix(part, ".rels") {
					if err := wellFormed(data); err != nil {
						t.Errorf("%s is not well-formed XML: %v", part, err)
					}
				}
			}

			types := string(parts["[Content_Types].xml"])
			for part := range parts {
				ext := path.Ext(part)
				if part == "[Content_Types].xml" {
					continue
				}
				if !strings.Contains(types, `PartName="/`+part+`"`) && !strings.Contains(types, `Extension="`+strings.TrimPrefix(ext, ".")+`"`) {
					t.Errorf("%s has no content type", part)
				}
			}
			for _, m := range regexp.MustCompile(`PartName="/([^"]+)"`).FindAllStringSubmatch(types, -1) {
				if _, ok := parts[m[1]]; !ok {
					t.Errorf("[Content_Types].xml overrides %s, which does not exist", m[1])
				}
			}

			target := regexp.MustCompile(`Target="([^"]+)"`)
			for part, data := range parts {
				if !strings.HasSuffix(part, ".rels") {
					continue
				}
				// A part's rels live at dir/_rels/name.rels; targets are
				// relative to dir.
				base := path.Dir(path.Dir(part))
				for _, m := range target.FindAllStringSubmatch(string(data), -1) {
					resolved := path.Clean(path.Join(base, m[1]))
					if _, ok := parts[resolved]; !ok {
						t.Errorf("%s points at %s (%s), which does not exist", part, m[1], resolved)
					}
				}
			}
		})
	}
}

func wellFormed(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func TestWritePPTX_RejectsAnEmptyDeck(t *testing.T) {
	if err := writePPTX(filepath.Join(t.TempDir(), "x.pptx"), pptxDeck{size: slideSizes[0]}); err == nil {
		t.Error("writePPTX accepted a deck with no slides")
	}
}

// TestWritePPTX_FailureLeavesNoFile checks the package is assembled beside
// the destination and only renamed into place once complete, so a failure
// part way through never leaves a truncated .pptx behind.
func TestWritePPTX_FailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "deck.pptx")
	deck := pptxDeck{size: slideSizes[0], slides: []pptxSlide{
		{pngPath: fakeSlidePNG(t, 4, 4)},
		{pngPath: filepath.Join(dir, "missing.png")},
	}}
	if err := writePPTX(out, deck); err == nil {
		t.Fatal("writePPTX succeeded with a missing slide image")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		t.Errorf("left behind %s", e.Name())
	}
}

func TestWritePPTX_CopiesTheImageBytes(t *testing.T) {
	img := fakeSlidePNG(t, 8, 8)
	want, err := os.ReadFile(img)
	if err != nil {
		t.Fatalf("read png: %v", err)
	}
	parts := writeTestDeck(t, pptxDeck{size: slideSizes[0], slides: []pptxSlide{{pngPath: img}}})
	if !bytes.Equal(parts["ppt/media/image1.png"], want) {
		t.Error("the embedded image differs from the captured file")
	}
}
