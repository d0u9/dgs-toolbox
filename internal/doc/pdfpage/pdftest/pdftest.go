// Package pdftest makes PDFs for tests: small, valid, and built here, so
// no fixture file is kept.
package pdftest

import (
	"bytes"
	"fmt"
)

// Made is a computer-made PDF: one A4 portrait page, turned by rotate,
// holding a line of text and no image, as an office program writes one.
func Made(rotate int) []byte {
	content := "BT /F1 24 Tf 72 720 Td (Hello) Tj ET"
	return build([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Rotate %d /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>", rotate),
		stream(content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})
}

// FormBox is where Form's filled field lies, in points from the page's
// lower left: x, y, width, height.
var FormBox = [4]int{100, 600, 200, 100}

// Form is a filled form: one A4 page whose content draws nothing, and one
// text field whose appearance fills FormBox black, as a form program writes
// what was typed into a field.
func Form() []byte {
	x, y, w, h := FormBox[0], FormBox[1], FormBox[2], FormBox[3]
	return build([]string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Annots [5 0 R] >>",
		stream(""),
		fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Tx /T (f) /V (x) /Rect [%d %d %d %d] /P 3 0 R /AP << /N 6 0 R >> >>", x, y, x+w, y+h),
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 %d %d] /Length %d >>\nstream\n%s\nendstream", w, h, len(fill(w, h)), fill(w, h)),
	})
}

func fill(w, h int) string { return fmt.Sprintf("0 g 0 0 %d %d re f", w, h) }

func stream(content string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
}

// build numbers objs from 1 and writes them with their cross-reference
// table; the first is the catalog.
func build(objs []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}
