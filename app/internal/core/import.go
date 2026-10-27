package core

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

type excelCell struct {
	Ref    string `xml:"r,attr"`
	Type   string `xml:"t,attr"`
	Value  string `xml:"v"`
	Inline struct {
		Text string `xml:"t"`
	} `xml:"is"`
}
type excelRow struct {
	Number int         `xml:"r,attr"`
	Cells  []excelCell `xml:"c"`
}
type excelSheet struct {
	Rows []excelRow `xml:"sheetData>row"`
}

func excelDate(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if t, e := ParseDate(v); e == nil {
		return t.Format("2006-01-02"), nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > 2958465 {
		return "", fmt.Errorf("fecha Excel no válida: %s", v)
	}
	return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n).Format("2006-01-02"), nil
}

// ParseExcel importa únicamente las hojas de datos, nunca el documento Word.
func ParseExcel(data []byte) ([]Diver, []Dive, error) {
	zr, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return nil, nil, e
	}
	files := map[string][]byte{}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > maxBackupBytes {
			return nil, nil, fmt.Errorf("Excel demasiado grande")
		}
		if f.UncompressedSize64 > 32<<20 {
			return nil, nil, fmt.Errorf("hoja demasiado grande")
		}
		r, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		b, err := io.ReadAll(io.LimitReader(r, 32<<20+1))
		r.Close()
		if err != nil {
			return nil, nil, err
		}
		files[f.Name] = b
	}
	var shared struct {
		Items []struct {
			Text string `xml:"t"`
			Runs []struct {
				Text string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if b := files["xl/sharedStrings.xml"]; len(b) > 0 {
		if e = xml.Unmarshal(b, &shared); e != nil {
			return nil, nil, e
		}
	}
	ss := []string{}
	for _, i := range shared.Items {
		v := i.Text
		for _, r := range i.Runs {
			v += r.Text
		}
		ss = append(ss, v)
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
		Properties struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
	}
	if e = xml.Unmarshal(files["xl/workbook.xml"], &wb); e != nil {
		return nil, nil, e
	}
	if wb.Properties.Date1904 == "1" || wb.Properties.Date1904 == "true" {
		return nil, nil, fmt.Errorf("calendario Excel 1904 no admitido")
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if e = xml.Unmarshal(files["xl/_rels/workbook.xml.rels"], &rels); e != nil {
		return nil, nil, e
	}
	targets := map[string]string{}
	for _, r := range rels.Items {
		t := path.Join("xl", r.Target)
		if strings.HasPrefix(r.Target, "/") {
			t = strings.TrimPrefix(r.Target, "/")
		}
		targets[r.ID] = t
	}
	sheets := map[string][]map[string]string{}
	for _, sh := range wb.Sheets {
		if sh.Name != "BUCEADORES" && sh.Name != "INMERSIONES" {
			continue
		}
		var sheet excelSheet
		if e = xml.Unmarshal(files[targets[sh.ID]], &sheet); e != nil {
			return nil, nil, e
		}
		rows := []map[string]string{}
		for _, r := range sheet.Rows {
			if r.Number < 5 {
				continue
			}
			row := map[string]string{}
			for _, c := range r.Cells {
				col := strings.TrimRight(c.Ref, "0123456789")
				v := c.Value
				if c.Type == "s" {
					i, err := strconv.Atoi(v)
					if err != nil || i < 0 || i >= len(ss) {
						return nil, nil, fmt.Errorf("cadena Excel no válida")
					}
					v = ss[i]
				} else if c.Type == "inlineStr" {
					v = c.Inline.Text
				}
				row[col] = strings.TrimSpace(v)
			}
			rows = append(rows, row)
		}
		sheets[sh.Name] = rows
	}
	if sheets["BUCEADORES"] == nil || sheets["INMERSIONES"] == nil {
		return nil, nil, fmt.Errorf("faltan hojas BUCEADORES / INMERSIONES")
	}
	yes := func(v string) bool { return strings.EqualFold(v, "SI") || strings.EqualFold(v, "SÍ") || v == "1" }
	divers := []Diver{}
	dives := []Dive{}
	for _, r := range sheets["BUCEADORES"] {
		if r["C"] == "" {
			continue
		}
		date, err := excelDate(r["F"])
		if err != nil {
			return nil, nil, err
		}
		divers = append(divers, Diver{ID: r["A"], Rank: r["B"], Name: r["C"], EntryDate: date, QualBasic: yes(r["G"]), QualSDSAir: yes(r["H"]), QualSDSHeliox: yes(r["I"]), Active: yes(r["J"]), Notes: r["L"]})
	}
	for row, r := range sheets["INMERSIONES"] {
		if r["D"] == "" {
			continue
		}
		date, err := excelDate(r["D"])
		if err != nil {
			return nil, nil, err
		}
		depth := 0.0
		if r["I"] != "" {
			depth, err = strconv.ParseFloat(strings.ReplaceAll(r["I"], ",", "."), 64)
			if err != nil {
				return nil, nil, err
			}
		}
		bt := 0
		if r["J"] != "" {
			bt, err = strconv.Atoi(r["J"])
			if err != nil {
				return nil, nil, err
			}
		}
		q := strings.ToLower(r["E"])
		gas := strings.ToUpper(r["G"])
		dt := ""
		switch {
		case strings.Contains(q, "sds aire") && gas == "AIRE":
			dt = "SDS Aire"
		case strings.Contains(q, "sds heliox"):
			dt = "SDFS Heliox"
		case (strings.Contains(q, "autónomo") || strings.Contains(q, "autonomo")) && gas == "AIRE":
			dt = "Autónomo Aire"
		case (strings.Contains(q, "autónomo") || strings.Contains(q, "autonomo")) && strings.Contains(gas, "NITROX"):
			dt = "Autónomo Nitrox"
		default:
			return nil, nil, fmt.Errorf("fila %d: no se puede deducir Dato IB sin ambigüedad", row+5)
		}
		dives = append(dives, Dive{ID: len(dives) + 1, SourceRecord: fmt.Sprintf("excel:%d:%s", row+5, r["A"]), DiverID: r["C"], Date: date, Qualification: r["E"], BreathingMode: r["F"], Gas: r["G"], SurfaceSupply: yes(r["H"]), MaxDepth: depth, BottomTime: bt, DiveType: dt, IBCode: IBCodeFor(dt), Purpose: r["K"], Notes: r["L"], Valid: yes(r["M"])})
	}
	if e = ValidateStore(Store{SchemaVersion: SchemaVersion, Divers: divers, Dives: dives}); e != nil {
		return nil, nil, e
	}
	return divers, dives, nil
}
func sameDive(a, b Dive) bool {
	a.ID = 0
	b.ID = 0
	a.SourceRecord = ""
	b.SourceRecord = ""
	a.CreatedAt = ""
	b.CreatedAt = ""
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func (a *App) importExcel(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST", 405)
		return
	}
	if !a.csrfOK(r) {
		http.Error(w, "CSRF", 403)
		return
	}
	f, _, err := r.FormFile("excel")
	if err != nil {
		http.Error(w, "Excel requerido", 400)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ds, dives, err := ParseExcel(b)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err = a.autoBackup("antes de importar Excel"); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	err = a.Repo.Update(func(s *Store) error {
		for _, d := range ds {
			if old := s.DiverByID(d.ID); old != nil {
				if old.Name != d.Name {
					return fmt.Errorf("ID %s ya pertenece a otro nombre; importación cancelada", d.ID)
				}
				continue
			}
			s.Divers = append(s.Divers, d)
		}
		for _, d := range dives {
			duplicate := false
			for _, old := range s.Dives {
				if sameDive(d, old) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			d.ID = s.NextDiveID
			s.NextDiveID++
			d.CreatedAt = nowUTC()
			s.Dives = append(s.Dives, d)
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/inmersiones", 302)
}
