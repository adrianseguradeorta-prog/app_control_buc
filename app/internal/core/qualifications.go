package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func MaintenanceFor(diver Diver, dives []Dive, now time.Time) []QualState {
	return []QualState{
		calcQual(diver.QualBasic, "basic", "Autónomo", 3, "3 inmersiones, una entre 25 y 30 m", diver, dives, now),
		calcQual(diver.QualSDSAir, "sds_air", "SDS Aire", 2, "2 inmersiones", diver, dives, now),
		calcQual(diver.QualSDSHeliox, "sds_heliox", "SDS HELIOX", 3, "2 con aire y 1 con mezcla a más de 40 m", diver, dives, now),
	}
}

func calcQual(assigned bool, code, label string, required int, recovery string, diver Diver, dives []Dive, now time.Time) QualState {
	s := QualState{Code: code, Label: label, Required: required, Recovery: recovery, Status: "NO ASIGNADA"}
	if !assigned {
		return s
	}
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cut := now.AddDate(-1, 0, 0)
	var all, recent []Dive
	for _, d := range dives {
		if d.DiverID != diver.ID || !d.Valid || d.MaxDepth <= 0 || d.BottomTime <= 0 {
			continue
		}
		if !matchesQual(code, d) {
			continue
		}
		t, err := ParseDate(d.Date)
		if err != nil || t.After(now) {
			continue
		}
		if code == "sds_air" && (!d.SurfaceSupply || !strings.EqualFold(strings.TrimSpace(d.Gas), "AIRE")) {
			continue
		}
		all = append(all, d)
		if !t.Before(cut) {
			recent = append(recent, d)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Date > all[j].Date })
	sort.Slice(recent, func(i, j int) bool { return recent[i].Date > recent[j].Date })
	s.Dives = recent
	s.Count12m = len(recent)
	if len(all) > 0 {
		s.LastDive = all[0].Date
		if t, err := ParseDate(all[0].Date); err == nil {
			s.Deadline = t.AddDate(1, 0, 0).Format("2006-01-02")
		}
	}
	if len(all) == 0 {
		s.Status = "SIN DATOS"
		s.Detail = "No hay inmersiones válidas para esta cualificación."
		return s
	}
	specific := specificOK(code, recent)
	s.SpecificOK = specific
	if len(recent) >= required && specific {
		s.Status = "OK"
		s.Detail = "Requisitos de mantenimiento cumplidos."
		return s
	}
	last, _ := ParseDate(all[0].Date)
	if now.Sub(last) > 2*365*24*time.Hour {
		s.Status = "FUERA DE PLAZO"
	} else {
		s.Status = "A RECUPERAR"
	}
	s.Detail = fmt.Sprintf("%d/%d inmersiones válidas en 12 meses; requisito específico: %s.", len(recent), required, map[bool]string{true: "cumplido", false: "pendiente"}[specific])
	return s
}

func matchesQual(code string, d Dive) bool {
	q := strings.ToLower(d.Qualification)
	switch code {
	case "basic":
		return strings.Contains(q, "autónomo") || strings.Contains(q, "autonomo")
	case "sds_air":
		return strings.Contains(q, "sds aire")
	case "sds_heliox":
		return strings.Contains(q, "sds heliox")
	}
	return false
}

func specificOK(code string, ds []Dive) bool {
	switch code {
	case "basic":
		for _, d := range ds {
			if d.MaxDepth >= 25 && d.MaxDepth <= 30 {
				return true
			}
		}
		return false
	case "sds_air":
		for _, d := range ds {
			if d.SurfaceSupply && strings.EqualFold(strings.TrimSpace(d.Gas), "AIRE") {
				return true
			}
		}
		return false
	case "sds_heliox":
		air, mix := 0, 0
		for _, d := range ds {
			if strings.EqualFold(strings.TrimSpace(d.Gas), "AIRE") {
				air++
			}
			if gas := strings.TrimSpace(d.Gas); d.MaxDepth > 40 && gas != "" && !strings.EqualFold(gas, "AIRE") {
				mix++
			}
		}
		return air >= 2 && mix >= 1
	}
	return false
}
