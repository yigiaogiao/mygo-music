package hanzi

import "testing"

func TestToSimplified(t *testing.T) {
	tests := []struct{ in, want string }{
		{"為什麼失眠的聲音", "为什么失眠的声音"},
		{"下雨天了怎麼辦", "下雨天了怎么办"},
		{"李誌", "李志"},
		{"優の良曲 南搞小孩", "优の良曲 南搞小孩"},
		{"妳和祢", "你和祢"},
		{"already 简体", "already 简体"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ToSimplified(tt.in); got != tt.want {
			t.Errorf("ToSimplified(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
