package attachments

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// validateOOXML 检查 OOXML 容器结构，拒绝带宏的 Office 文件。
//
// 规则：
//   - 必须是有效 ZIP
//   - 必须包含 [Content_Types].xml
//   - docx/xlsx/pptx 对应 word/、xl/、ppt/ 根目录必须存在
//   - 任一处出现 vbaProject.bin 即拒绝
func validateOOXML(f *os.File, ext string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(f, info.Size())
	if err != nil {
		return typeNotAllowedError("invalid OOXML container: " + err.Error())
	}
	hasContentTypes := false
	hasExpectedDir := false
	expectedDir := ""
	switch ext {
	case ".docx":
		expectedDir = "word/"
	case ".xlsx":
		expectedDir = "xl/"
	case ".pptx":
		expectedDir = "ppt/"
	}
	for _, file := range reader.File {
		name := file.Name
		if name == "[Content_Types].xml" {
			hasContentTypes = true
		}
		if expectedDir != "" && strings.HasPrefix(name, expectedDir) {
			hasExpectedDir = true
		}
		if strings.HasSuffix(name, "vbaProject.bin") {
			return typeNotAllowedError("macro-enabled Office file is not allowed")
		}
	}
	if !hasContentTypes {
		return typeNotAllowedError("OOXML missing [Content_Types].xml")
	}
	if expectedDir != "" && !hasExpectedDir {
		return typeNotAllowedError(fmt.Sprintf("OOXML missing %s directory", expectedDir))
	}
	return nil
}

// validateArchiveHeader 只校验容器 header，不解压。
func validateArchiveHeader(f *os.File, ext string) error {
	header := make([]byte, 8)
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	header = header[:n]
	switch ext {
	case ".zip", ".7z":
		// ZIP magic "PK\x03\x04"；7z magic "7z\xbc\xaf\x27\x1c"
		if bytes.HasPrefix(header, []byte("PK\x03\x04")) {
			return nil
		}
		if ext == ".7z" && bytes.HasPrefix(header, []byte("7z\xbc\xaf\x27\x1c")) {
			return nil
		}
		return typeNotAllowedError("archive header mismatch for " + ext)
	case ".gz", ".tgz":
		// gzip magic 0x1f 0x8b
		if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
			return nil
		}
		return typeNotAllowedError("archive header mismatch for " + ext)
	case ".tar":
		// tar 在 offset 257 处有 "ustar"；只校验文件至少 512 字节且 ustar 存在。
		if info, err := f.Stat(); err == nil && info.Size() >= 512 {
			ustar := make([]byte, 5)
			if _, err := f.ReadAt(ustar, 257); err == nil && bytes.Equal(ustar, []byte("ustar")) {
				return nil
			}
		}
		return typeNotAllowedError("tar header mismatch")
	}
	return nil
}

// fileSectionReader 占位：旧实现使用 zip.NewReader 需要的 ReaderAt；
// 现在直接传 *os.File（os.File 自身实现 io.ReaderAt）。
