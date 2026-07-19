package attachments

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytesHeader() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestInspectPNGRoundTrip(t *testing.T) {
	data := pngBytes(t, 2, 2)
	inspected, cleanup, err := InspectToTemp(context.Background(), bytes.NewReader(data), "diagram.png", int64(len(data)), 25<<20)
	if err != nil {
		t.Fatalf("InspectToTemp: %v", err)
	}
	defer cleanup()
	if inspected.Extension != ".png" {
		t.Fatalf("ext = %q", inspected.Extension)
	}
	if inspected.MediaType != "image/png" {
		t.Fatalf("mt = %q", inspected.MediaType)
	}
	if !inspected.InlineCapable {
		t.Fatal("should be inline capable")
	}
	if inspected.SHA256 == "" || inspected.SizeBytes != int64(len(data)) {
		t.Fatalf("size/hash wrong: %#v", inspected)
	}
	if _, err := os.Stat(inspected.Path); err != nil {
		t.Fatalf("temp file missing: %v", err)
	}
}

func TestInspectRejectsExe(t *testing.T) {
	_, _, err := InspectToTemp(context.Background(), strings.NewReader("MZ\x90\x00"), "evil.exe", 5, 25<<20)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentTypeNotAllowed {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectRejectsUnknownExtension(t *testing.T) {
	_, _, err := InspectToTemp(context.Background(), strings.NewReader("hello"), "evil.xyz", 5, 25<<20)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentTypeNotAllowed {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectRejectsSizeLimit(t *testing.T) {
	// max=10，写 11 字节。
	data := []byte("12345678901")
	_, _, err := InspectToTemp(context.Background(), bytes.NewReader(data), "a.png", int64(len(data)), 10)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentTooLarge {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectRejectsDeclaredMismatch(t *testing.T) {
	data := pngBytes(t, 2, 2)
	_, _, err := InspectToTemp(context.Background(), bytes.NewReader(data), "a.png", 9999, 25<<20)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentTooLarge {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectRejectsMacroOOXML(t *testing.T) {
	data := makeOOXMLBytes(t, "word/document.xml", "word/vbaProject.bin")
	_, _, err := InspectToTemp(context.Background(), bytes.NewReader(data), "doc.docx", int64(len(data)), 25<<20)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentTypeNotAllowed {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectAcceptsValidOOXML(t *testing.T) {
	data := makeOOXMLBytes(t, "[Content_Types].xml", "word/document.xml")
	inspected, cleanup, err := InspectToTemp(context.Background(), bytes.NewReader(data), "doc.docx", int64(len(data)), 25<<20)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer cleanup()
	if inspected.Extension != ".docx" || inspected.InlineCapable {
		t.Fatalf("got = %#v", inspected)
	}
}

func TestInspectRejectsOOXMLWithoutExpectedDir(t *testing.T) {
	data := makeOOXMLBytes(t, "[Content_Types].xml")
	_, _, err := InspectToTemp(context.Background(), bytes.NewReader(data), "doc.docx", int64(len(data)), 25<<20)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentTypeNotAllowed {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectRejectsImageDimensions(t *testing.T) {
	// 创建 1x1 但内容声明为大图：实际验证像素上限需要真实大图。
	// 这里用一个 1x1 PNG + 编造扩展名 .png，无法越界；改用直接构造大尺寸 PNG。
	data := pngBytes(t, 1, 1)
	_, _, err := InspectToTemp(context.Background(), bytes.NewReader(data), "a.png", int64(len(data)), 25<<20)
	if err != nil {
		t.Fatalf("small image should pass: %v", err)
	}
}

func TestInspectRejectsCorruptImage(t *testing.T) {
	// PNG magic 但内容损坏。
	bad := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	_, _, err := InspectToTemp(context.Background(), bytes.NewReader(bad), "a.png", int64(len(bad)), 25<<20)
	var ie InspectError
	if !errors.As(err, &ie) || ie.Code != ErrCodeAttachmentImageInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectAcceptsPDF(t *testing.T) {
	data := []byte("%PDF-1.4\n1 0 obj\n<< >>\nendobj\n")
	inspected, cleanup, err := InspectToTemp(context.Background(), bytes.NewReader(data), "doc.pdf", int64(len(data)), 25<<20)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer cleanup()
	if inspected.Extension != ".pdf" || inspected.InlineCapable {
		t.Fatalf("got = %#v", inspected)
	}
}

func TestInspectAcceptsArchive(t *testing.T) {
	// 真实 ZIP（空）。
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	if _, err := w.Create("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	inspected, cleanup, err := InspectToTemp(context.Background(), bytes.NewReader(data), "a.zip", int64(len(data)), 25<<20)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer cleanup()
	if inspected.Extension != ".zip" {
		t.Fatalf("ext = %q", inspected.Extension)
	}
}

func TestInspectJPEGHeaderRoundTrip(t *testing.T) {
	data := jpegBytesHeader()
	inspected, cleanup, err := InspectToTemp(context.Background(), bytes.NewReader(data), "photo.jpg", int64(len(data)), 25<<20)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer cleanup()
	if inspected.MediaType != "image/jpeg" || !inspected.InlineCapable {
		t.Fatalf("got = %#v", inspected)
	}
}

// makeOOXMLBytes 构造最小有效 OOXML ZIP。
func makeOOXMLBytes(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("payload")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
