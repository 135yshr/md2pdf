package converter

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/iterm2"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/charmbracelet/x/ansi/sixel"
)

// Mermaid rendering modes for console output, selected with -mermaid-render.
const (
	// MermaidRenderAuto draws diagrams as inline images when the terminal
	// supports an image protocol and falls back to the Mermaid source
	// otherwise.
	MermaidRenderAuto = "auto"
	// MermaidRenderImage forces inline images and fails when the terminal
	// cannot display them.
	MermaidRenderImage = "image"
	// MermaidRenderSource always prints the Mermaid source as a code block.
	MermaidRenderSource = "source"
)

// mermaidRenderModes lists the values accepted by -mermaid-render.
var mermaidRenderModes = []string{MermaidRenderAuto, MermaidRenderImage, MermaidRenderSource}

// ValidateMermaidRenderMode reports whether value names a supported Mermaid
// rendering mode for console output. An empty value selects the default.
func ValidateMermaidRenderMode(value string) error {
	if value == "" || slices.Contains(mermaidRenderModes, value) {
		return nil
	}
	return fmt.Errorf("unknown Mermaid render mode %q: use one of %s",
		value, strings.Join(mermaidRenderModes, ", "))
}

// terminalImageProtocol identifies the inline-image escape sequence family a
// terminal understands.
type terminalImageProtocol int

// Inline image protocols md2pdf can emit: none for a terminal that cannot show
// images, the kitty graphics protocol (APC G), the iTerm2 inline image protocol
// (OSC 1337), and the DEC Sixel raster protocol (DCS q).
const (
	imageProtocolNone terminalImageProtocol = iota
	imageProtocolKitty
	imageProtocolITerm2
	imageProtocolSixel
)

// String implements fmt.Stringer.
func (p terminalImageProtocol) String() string {
	switch p {
	case imageProtocolKitty:
		return "kitty"
	case imageProtocolITerm2:
		return "iTerm2"
	case imageProtocolSixel:
		return "sixel"
	case imageProtocolNone:
		return "none"
	default:
		return "unknown"
	}
}

// sixelTerms lists TERM values for terminals that speak Sixel but advertise no
// other image protocol. Detection is environment-based on purpose: querying the
// terminal with DA1 would require putting it into raw mode and waiting for a
// reply, which is too invasive for a document renderer.
var sixelTerms = []string{"foot", "foot-extra", "mlterm", "mlterm-256color", "yaft", "yaft-256color"}

// detectImageProtocol picks the inline image protocol to use based on the
// terminal-identifying environment variables read through getenv. Kitty is
// preferred because terminals that speak it tend to also set TERM_PROGRAM
// values matching other protocols.
func detectImageProtocol(getenv func(string) string) terminalImageProtocol {
	termName := strings.ToLower(getenv("TERM"))
	termProgram := strings.ToLower(getenv("TERM_PROGRAM"))

	switch {
	case strings.Contains(termName, "kitty"),
		strings.Contains(termName, "ghostty"),
		termProgram == "ghostty",
		getenv("KITTY_WINDOW_ID") != "":
		return imageProtocolKitty
	case termProgram == "iterm.app", termProgram == "wezterm":
		return imageProtocolITerm2
	case strings.Contains(termName, "sixel"), slices.Contains(sixelTerms, termName):
		return imageProtocolSixel
	default:
		return imageProtocolNone
	}
}

// consoleMermaidPlan records how Mermaid blocks should be rendered for a single
// console run.
type consoleMermaidPlan struct {
	// mode is the resolved rendering mode, never empty and never "auto".
	mode string
	// protocol is the image protocol to emit, set only when mode is
	// MermaidRenderImage.
	protocol terminalImageProtocol
}

// emitsImages reports whether the plan will write image escape sequences.
func (p consoleMermaidPlan) emitsImages() bool {
	return p.mode == MermaidRenderImage && p.protocol != imageProtocolNone
}

// resolveConsoleMermaidPlan decides how Mermaid blocks are rendered. Images
// need an interactive terminal with a known image protocol and color enabled;
// "auto" degrades to the Mermaid source whenever any of that is missing, while
// an explicit "image" reports which capability was absent.
func resolveConsoleMermaidPlan(mode string, isTTY, noColor bool, getenv func(string) string) (consoleMermaidPlan, error) {
	if mode == "" {
		mode = MermaidRenderAuto
	}
	sourcePlan := consoleMermaidPlan{mode: MermaidRenderSource}
	if mode == MermaidRenderSource {
		return sourcePlan, nil
	}

	if !isTTY || noColor {
		if mode == MermaidRenderImage {
			return consoleMermaidPlan{}, errors.New(
				"-mermaid-render image needs an interactive terminal to draw inline images, " +
					"but the output is redirected or NO_COLOR is set")
		}
		return sourcePlan, nil
	}

	if getenv == nil {
		getenv = os.Getenv
	}
	protocol := detectImageProtocol(getenv)
	if protocol == imageProtocolNone {
		if mode == MermaidRenderImage {
			return consoleMermaidPlan{}, errors.New(
				"-mermaid-render image needs a terminal with an inline image protocol " +
					"(kitty, iTerm2 or Sixel), but none was detected from TERM and TERM_PROGRAM")
		}
		return sourcePlan, nil
	}

	return consoleMermaidPlan{mode: MermaidRenderImage, protocol: protocol}, nil
}

// consoleDiagram pairs the placeholder left in the Markdown with the terminal
// escape sequence that replaces it once glamour has rendered the document.
type consoleDiagram struct {
	// placeholder is the token standing in for the diagram in the Markdown.
	placeholder string
	// content is the text (usually an escape sequence) to splice in.
	content string
}

// consoleTokenPrefix is the prefix of the stand-in token left in the Markdown
// for a diagram that will be drawn as an image. It is deliberately much shorter
// than mermaidPlaceholderPrefix: glamour hard-wraps a word that does not fit the
// wrap width, and a token split across two lines could no longer be found.
const consoleTokenPrefix = "MD2PDFDG"

// consoleTokenRe matches a console diagram token. Matching the whole token
// rather than searching for each token in turn keeps token 1 from being found
// inside token 10.
var consoleTokenRe = regexp.MustCompile(consoleTokenPrefix + `\d+`)

// consoleDiagramToken returns the stand-in token for the idx-th diagram.
func consoleDiagramToken(idx int) string {
	return fmt.Sprintf("%s%d", consoleTokenPrefix, idx)
}

// consoleTokenFits reports whether a token of the given length survives glamour
// at this wrap width. Glamour indents the document and hard-wraps any word wider
// than the remaining space, so a token that does not fit would be split across
// lines and become unfindable.
func consoleTokenFits(tokenLen, width int) bool {
	return width >= tokenLen+consoleTokenWidthMargin
}

// consoleTokenWidthMargin is the room left for glamour's document indent when
// checking that a diagram token fits on one line.
const consoleTokenWidthMargin = 4

// spliceConsoleDiagrams replaces every line of rendered output holding a diagram
// token with the diagram for that token, indented to the column glamour placed
// the token at. The whole line is dropped so the styling glamour wrapped the
// token in does not leak around the diagram. Tokens that could not be found are
// returned so the caller can report them instead of losing a diagram silently.
func spliceConsoleDiagrams(rendered string, diagrams []consoleDiagram) (string, []string) {
	if len(diagrams) == 0 {
		return rendered, nil
	}

	byToken := make(map[string]string, len(diagrams))
	for _, d := range diagrams {
		byToken[d.placeholder] = d.content
	}
	placed := make(map[string]bool, len(diagrams))

	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		stripped := ansi.Strip(line)
		token := consoleTokenRe.FindString(stripped)
		content, ok := byToken[token]
		if token == "" || !ok {
			out = append(out, line)
			continue
		}
		placed[token] = true
		indent := stripped[:len(stripped)-len(strings.TrimLeft(stripped, " "))]
		for _, contentLine := range strings.Split(content, "\n") {
			out = append(out, indent+contentLine)
		}
	}

	var missing []string
	for _, d := range diagrams {
		if !placed[d.placeholder] {
			missing = append(missing, d.placeholder)
		}
	}
	return strings.Join(out, "\n"), missing
}

// prepareConsoleMermaid rewrites the Markdown so glamour never sees a Mermaid
// block that will be drawn as an image, and returns the diagrams to splice in
// afterwards. The original bytes are returned untouched whenever no diagram will
// be drawn, so output stays identical to the source-only rendering. Diagrams are
// rendered independently: one failure falls back to that block's source and
// leaves the rest of the document alone.
func (c *Converter) prepareConsoleMermaid(md []byte, plan consoleMermaidPlan, width int) ([]byte, []consoleDiagram, error) {
	if !plan.emitsImages() {
		return md, nil, nil
	}

	markdown, blocks := extractMermaidFromMarkdown(string(md))
	if len(blocks) == 0 {
		return md, nil, nil
	}

	// Every token must survive glamour on a single line, so bail out to source
	// output rather than risk leaking a hard-wrapped token into the document.
	if longest := len(consoleDiagramToken(len(blocks) - 1)); !consoleTokenFits(longest, width) {
		c.logf("Width %d is too narrow to place diagrams; Mermaid blocks stay as source.", width)
		return md, nil, nil
	}

	if !c.mermaidAvailable() {
		c.logf("mmdc not found; Mermaid diagrams stay as source " +
			"(install @mermaid-js/mermaid-cli to draw them as images)")
		return md, nil, nil
	}

	c.logf("Drawing %d Mermaid diagram(s) as %s images...", len(blocks), plan.protocol)
	diagrams := make([]consoleDiagram, 0, len(blocks))
	for i, block := range blocks {
		sequence, err := c.renderConsoleDiagram(i, block.Source, plan.protocol, width)
		if err != nil {
			c.logf("  diagram %d could not be drawn (%v); showing its source instead", i, err)
			markdown = strings.Replace(markdown, block.Placeholder, fencedMermaid(block.Source), 1)
			continue
		}
		c.logf("  diagram %d drawn (%d bytes of %s escape sequence)", i, len(sequence), plan.protocol)
		token := consoleDiagramToken(i)
		markdown = strings.Replace(markdown, block.Placeholder, token, 1)
		diagrams = append(diagrams, consoleDiagram{placeholder: token, content: sequence})
	}
	return []byte(markdown), diagrams, nil
}

// renderConsoleDiagram rasterises one Mermaid block and encodes the PNG for the
// terminal.
func (c *Converter) renderConsoleDiagram(idx int, source string, protocol terminalImageProtocol, width int) (string, error) {
	pngPath, err := c.rasterizeMermaid(idx, source)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(pngPath)
	if err != nil {
		return "", fmt.Errorf("read rendered diagram: %w", err)
	}
	return encodeTerminalImage(protocol, data, width)
}

// consoleDiagramPNG rasterises a Mermaid block to a PNG and returns its
// absolute path. It is the production implementation behind
// Converter.rasterizeMermaid.
func (c *Converter) consoleDiagramPNG(idx int, source string) (string, error) {
	pcfg, err := c.ensurePuppeteerConfig()
	if err != nil {
		return "", fmt.Errorf("puppeteer config: %w", err)
	}
	rel, err := c.renderSingleDiagramPNG(idx, source, pcfg)
	if err != nil {
		return "", err
	}
	return filepath.Join(c.workDir, filepath.FromSlash(rel)), nil
}

// mmdcAvailable reports whether the configured mmdc binary can be found, so a
// missing Mermaid CLI degrades to source output instead of failing the run.
func (c *Converter) mmdcAvailable() bool {
	_, err := exec.LookPath(c.resolveMmdc())
	return err == nil
}

// fencedMermaid wraps Mermaid source back into a fenced code block, widening the
// fence so source containing backtick runs stays inside the block.
func fencedMermaid(source string) string {
	longest := 0
	for i := 0; i < len(source); {
		if source[i] != '`' {
			i++
			continue
		}
		n := 0
		for i < len(source) && source[i] == '`' {
			n++
			i++
		}
		if n > longest {
			longest = n
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	if !strings.HasSuffix(source, "\n") {
		source += "\n"
	}
	return fence + "mermaid\n" + source + fence
}

// consoleImageCellWidthPx is the assumed width of a terminal cell in pixels. The
// real size is only obtainable from a TIOCGWINSZ ioctl that most terminals leave
// zeroed, so this average decides when a diagram is too wide for the wrap width.
const consoleImageCellWidthPx = 10

// fitColumns returns the number of terminal columns a diagram of the given pixel
// width should occupy, clamped to maxColumns. The second result reports whether
// the diagram had to be shrunk, so images that already fit keep their natural
// size instead of being stretched across the terminal.
func fitColumns(pixelWidth, maxColumns int) (cols int, clamped bool) {
	natural := max(1, (pixelWidth+consoleImageCellWidthPx-1)/consoleImageCellWidthPx)
	if maxColumns > 0 && natural > maxColumns {
		return maxColumns, true
	}
	return natural, false
}

// encodeTerminalImage encodes a PNG diagram as an inline image escape sequence
// for the given protocol, scaled down to at most maxColumns terminal columns.
func encodeTerminalImage(protocol terminalImageProtocol, data []byte, maxColumns int) (string, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decode diagram PNG: %w", err)
	}
	cols, clamped := fitColumns(img.Bounds().Dx(), maxColumns)

	switch protocol {
	case imageProtocolKitty:
		opts := &kitty.Options{
			Action:       kitty.TransmitAndPut,
			Transmission: kitty.Direct,
			Format:       kitty.PNG,
			Quiet:        2,
			Chunk:        true,
		}
		if clamped {
			opts.Columns = cols
		}
		var buf bytes.Buffer
		if err := kitty.EncodeGraphics(&buf, img, opts); err != nil {
			return "", fmt.Errorf("encode kitty graphics: %w", err)
		}
		return buf.String(), nil

	case imageProtocolITerm2:
		file := iterm2.File{
			Inline:  true,
			Content: []byte(base64.StdEncoding.EncodeToString(data)),
		}
		if clamped {
			file.Width = iterm2.Cells(cols)
		}
		return ansi.ITerm2(file), nil

	case imageProtocolSixel:
		if clamped {
			img = downscaleImage(img, cols*consoleImageCellWidthPx)
		}
		var payload bytes.Buffer
		if err := (&sixel.Encoder{}).Encode(&payload, img); err != nil {
			return "", fmt.Errorf("encode sixel image: %w", err)
		}
		// p2=1 avoids the black bar terminals draw for p2=0.
		return ansi.SixelGraphics(0, 1, 0, payload.Bytes()), nil

	case imageProtocolNone:
		return "", errors.New("no inline image protocol available for this terminal")
	default:
		return "", fmt.Errorf("unsupported image protocol %q", protocol)
	}
}

// downscaleImage shrinks src to targetWidth pixels, preserving the aspect ratio.
// Each destination pixel averages the source pixels it covers, which suits the
// downscaling of an already-supersampled mmdc render better than picking a
// single nearest sample. Images at or below targetWidth are returned unchanged.
func downscaleImage(src image.Image, targetWidth int) image.Image {
	bounds := src.Bounds()
	if targetWidth <= 0 || bounds.Dx() <= targetWidth {
		return src
	}

	targetHeight := max(1, int(math.Round(float64(bounds.Dy())*float64(targetWidth)/float64(bounds.Dx()))))
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	for y := range targetHeight {
		y0 := bounds.Min.Y + y*bounds.Dy()/targetHeight
		y1 := max(y0+1, bounds.Min.Y+(y+1)*bounds.Dy()/targetHeight)
		for x := range targetWidth {
			x0 := bounds.Min.X + x*bounds.Dx()/targetWidth
			x1 := max(x0+1, bounds.Min.X+(x+1)*bounds.Dx()/targetWidth)

			var sumR, sumG, sumB, sumA, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := src.At(sx, sy).RGBA()
					sumR += uint64(r)
					sumG += uint64(g)
					sumB += uint64(b)
					sumA += uint64(a)
					n++
				}
			}
			dst.Set(x, y, color.RGBA64{
				R: uint16(sumR / n), //nolint:gosec // G115: the mean of uint16 samples fits in uint16
				G: uint16(sumG / n), //nolint:gosec // G115: the mean of uint16 samples fits in uint16
				B: uint16(sumB / n), //nolint:gosec // G115: the mean of uint16 samples fits in uint16
				A: uint16(sumA / n), //nolint:gosec // G115: the mean of uint16 samples fits in uint16
			})
		}
	}
	return dst
}
