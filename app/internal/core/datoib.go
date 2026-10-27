package core

import "strings"

type IBOption struct{ Code, Label string }

var IBOptions = []IBOption{
	{"61001", "Autónomo Aire"},
	{"62001", "SDS Aire"},
	{"62002", "SDFS Heliox"},
	{"62003", "Autónomo Nitrox"},
	{"62004", "Autónomo Heliox/Trimix"},
	{"62005", "Semicerrado Nitrox (Buceo técnico)"},
	{"62006", "Semicerrado Trimix"},
	{"63001", "Cerrado Oxígeno"},
	{"64001", "Semicerrado Nitrox (Caza de minas)"},
	{"66001", "Control en Cámara"},
	{"66002", "Tratamiento O2"},
}

func IBCodeFor(label string) string {
	x := strings.TrimSpace(strings.ToLower(label))
	for _, o := range IBOptions {
		if strings.ToLower(o.Label) == x || o.Code == label {
			return o.Code
		}
	}
	return ""
}

func IBLabelFor(code string) string {
	for _, o := range IBOptions {
		if o.Code == code {
			return o.Label
		}
	}
	return ""
}
