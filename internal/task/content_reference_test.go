package task

import (
	"errors"
	"testing"
)

func TestParseContentReferencesAttachment(t *testing.T) {
	source := "![图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
	got, err := ParseContentReferences(source)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Kind != ContentReferenceAttachment {
		t.Fatalf("kind = %v", got[0].Kind)
	}
	if got[0].ID != "40af0185-316f-42bb-b52b-545d21f6f012" {
		t.Fatalf("id = %q", got[0].ID)
	}
	if !got[0].Image {
		t.Fatal("must be image")
	}
}

func TestParseContentReferencesAttachmentLink(t *testing.T) {
	source := "[需求.pdf](ref://attachment/a801f977-c745-4f47-95a4-7893a9317aba)"
	got, err := ParseContentReferences(source)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || got[0].Image {
		t.Fatalf("got = %#v", got)
	}
}

func TestParseContentReferencesRejectsMalformed(t *testing.T) {
	cases := []string{
		"[@A](REF://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)",
		"[@A](ref://user/8C8B1BED-2E75-4DE8-8D5F-C94CBF2B3001)",
		"[@A](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001?q=1)",
		"[P](ref://project/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)",
		"![](ref://attachment/not-a-uuid)",
		"[a](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012/extra)",
	}
	for _, source := range cases {
		t.Run(source, func(t *testing.T) {
			_, err := ParseContentReferences(source)
			if !errors.Is(err, ErrDescriptionReferenceInvalid) {
				t.Fatalf("err = %v, want ErrDescriptionReferenceInvalid", err)
			}
		})
	}
}

func TestParseContentReferencesIgnoresRegularLinks(t *testing.T) {
	source := "[example](https://example.com) and [mail](mailto:a@b.com)"
	got, err := ParseContentReferences(source)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %#v", got)
	}
}

func TestParseContentReferencesCodeBlockPseudoURI(t *testing.T) {
	// ref:// 出现在代码块内不应被解析为引用。
	source := "```\n[a](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)\n```"
	got, _ := ParseContentReferences(source)
	if len(got) != 0 {
		t.Fatalf("code block should not produce refs: %#v", got)
	}
}

func TestParseContentReferencesDeduplicatesAttachmentIDs(t *testing.T) {
	source := "![a](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)\n![a](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
	ids, err := AttachmentReferenceIDs(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids = %#v", ids)
	}
}
