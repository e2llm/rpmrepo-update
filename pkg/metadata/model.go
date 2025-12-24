package metadata

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Package represents a single package's metadata across primary/filelists/other.
type Package struct {
	Name          string
	Arch          string
	Epoch         int
	Version       string
	Release       string
	Summary       string
	Description   string
	License       string
	Vendor        string
	Group         string
	BuildHost     string
	SourceRPM     string
	URL           string
	Packager      string
	TimeBuild     int64
	TimeFile      int64
	SizePackage   uint64
	SizeInstalled uint64
	SizeArchive   uint64
	Location      string
	PkgID         string // checksum of RPM payload
	ChecksumType  string
	HeaderStart   int
	HeaderEnd     int
	Provides      []Relation
	Requires      []Relation
	Conflicts     []Relation
	Obsoletes     []Relation
	Files         []File
	Changelogs    []Changelog
}

func (p Package) NEVRA() string {
	epoch := p.Epoch
	epochPart := ""
	if epoch > 0 {
		epochPart = fmt.Sprintf("%d:", epoch)
	}
	return fmt.Sprintf("%s-%s%s-%s.%s", p.Name, epochPart, p.Version, p.Release, p.Arch)
}

type Relation struct {
	Name  string
	Flags string
	Epoch int
	Ver   string
	Rel   string
	Pre   bool
}

type File struct {
	Path string
	Type string // dir, ghost, or empty
}

type Changelog struct {
	Author string
	Date   int64
	Text   string
}

// ParsePackagesFromXML parses core metadata XML payloads (uncompressed) into Package structs.
func ParsePackagesFromXML(primaryXML, filelistsXML, otherXML []byte) ([]Package, error) {
	primary, err := parsePrimary(primaryXML)
	if err != nil {
		return nil, fmt.Errorf("parse primary: %w", err)
	}
	pkgs := make([]Package, 0, len(primary.Packages))
	index := make(map[string]*Package, len(primary.Packages))
	for _, p := range primary.Packages {
		pkg := packageFromPrimary(p)
		pkgs = append(pkgs, pkg)
		index[pkg.PkgID] = &pkgs[len(pkgs)-1]
	}

	if len(filelistsXML) > 0 {
		if err := parseFilelistsInto(index, filelistsXML); err != nil {
			return nil, fmt.Errorf("parse filelists: %w", err)
		}
	}
	if len(otherXML) > 0 {
		if err := parseOtherInto(index, otherXML); err != nil {
			return nil, fmt.Errorf("parse other: %w", err)
		}
	}
	return pkgs, nil
}

// RenderCoreXML renders primary/filelists/other XML payloads (uncompressed).
func RenderCoreXML(pkgs []Package) (primaryXML, filelistsXML, otherXML []byte, err error) {
	sorted := append([]Package(nil), pkgs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].NEVRA() < sorted[j].NEVRA()
	})
	primaryXML, err = marshalPrimary(sorted)
	if err != nil {
		return
	}
	filelistsXML, err = marshalFilelists(sorted)
	if err != nil {
		return
	}
	otherXML, err = marshalOther(sorted)
	return
}

// BuildCoreFilesFromPackages generates compressed core metadata files and checksum info.
func BuildCoreFilesFromPackages(pkgs []Package, checksumAlg string, now time.Time) ([]CoreFile, error) {
	checksumAlg = strings.ToLower(checksumAlg)
	if !SupportedChecksum(checksumAlg) {
		return nil, fmt.Errorf("unsupported checksum algorithm %q", checksumAlg)
	}
	primaryXML, filelistsXML, otherXML, err := RenderCoreXML(pkgs)
	if err != nil {
		return nil, err
	}
	type payload struct {
		name string
		data []byte
	}
	payloads := []payload{
		{"primary", primaryXML},
		{"filelists", filelistsXML},
		{"other", otherXML},
	}

	var coreFiles []CoreFile
	for _, p := range payloads {
		compressed, err := gzipBytes(p.data)
		if err != nil {
			return nil, err
		}
		sum, err := ComputeChecksum(compressed, checksumAlg)
		if err != nil {
			return nil, err
		}
		openSum, err := ComputeChecksum(p.data, checksumAlg)
		if err != nil {
			return nil, err
		}
		path := fmt.Sprintf("repodata/%s-%s.xml.gz", sum, p.name)
		coreFiles = append(coreFiles, CoreFile{
			Type:         p.name,
			Path:         path,
			Compressed:   compressed,
			Uncompressed: p.data,
			Checksum:     sum,
			OpenChecksum: openSum,
			Size:         int64(len(compressed)),
			OpenSize:     int64(len(p.data)),
			Timestamp:    now.Unix(),
		})
	}
	return coreFiles, nil
}

// UpdateRepoMDWithCore returns a new RepoMD using the provided core files,
// preserving existing non-core entries (excluding prestodelta).
func UpdateRepoMDWithCore(old RepoMD, core []CoreFile, checksumAlg string, now time.Time) RepoMD {
	newMD := RepoMD{
		Xmlns:    old.Xmlns,
		Revision: fmt.Sprintf("%d", now.Unix()),
	}
	if newMD.Xmlns == "" {
		newMD.Xmlns = RepoNamespace
	}
	for _, d := range old.Data {
		switch d.Type {
		case "primary", "filelists", "other", "prestodelta":
			continue
		default:
			newMD.Data = append(newMD.Data, d)
		}
	}
	for _, cf := range core {
		alg := checksumAlg
		if alg == "" {
			alg = "sha256"
		}
		openChecksum := cf.OpenChecksum
		newMD.Data = append(newMD.Data, RepoData{
			Type:         cf.Type,
			Checksum:     Checksum{Type: alg, Value: cf.Checksum},
			OpenChecksum: &Checksum{Type: alg, Value: openChecksum},
			Location:     Location{Href: cf.Path},
			Timestamp:    cf.Timestamp,
			Size:         cf.Size,
			OpenSize:     cf.OpenSize,
		})
	}
	return newMD
}

// Helpers and XML mapping structures.

type primaryXML struct {
	XMLName  xml.Name         `xml:"metadata"`
	Xmlns    string           `xml:"xmlns,attr"`
	XmlnsRpm string           `xml:"xmlns:rpm,attr"`
	Count    int              `xml:"packages,attr"`
	Packages []primaryPackage `xml:"package"`
}

type primaryPackage struct {
	Type        string         `xml:"type,attr"`
	Name        string         `xml:"name"`
	Arch        string         `xml:"arch"`
	Version     rpmVersion     `xml:"version"`
	Checksum    rpmPkgChecksum `xml:"checksum"`
	Summary     string         `xml:"summary"`
	Description string         `xml:"description"`
	Packager    string         `xml:"packager,omitempty"`
	URL         string         `xml:"url,omitempty"`
	Time        primaryTime    `xml:"time"`
	Size        primarySize    `xml:"size"`
	Location    Location       `xml:"location"`
	Format      primaryFormat  `xml:"format"`
}

type primaryTime struct {
	File  int64 `xml:"file,attr,omitempty"`
	Build int64 `xml:"build,attr,omitempty"`
}

type primarySize struct {
	Package   uint64 `xml:"package,attr"`
	Installed uint64 `xml:"installed,attr,omitempty"`
	Archive   uint64 `xml:"archive,attr,omitempty"`
}

type rpmPkgChecksum struct {
	Type  string `xml:"type,attr"`
	PkgID string `xml:"pkgid,attr"`
	Value string `xml:",chardata"`
}

type rpmVersion struct {
	Epoch string `xml:"epoch,attr,omitempty"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type primaryFormat struct {
	License     string       `xml:"rpm:license,omitempty"`
	Vendor      string       `xml:"rpm:vendor,omitempty"`
	Group       string       `xml:"rpm:group,omitempty"`
	BuildHost   string       `xml:"rpm:buildhost,omitempty"`
	SourceRPM   string       `xml:"rpm:sourcerpm,omitempty"`
	HeaderRange *headerRange `xml:"rpm:header-range,omitempty"`
	Provides    []depEntry   `xml:"rpm:provides>rpm:entry,omitempty"`
	Requires    []depEntry   `xml:"rpm:requires>rpm:entry,omitempty"`
	Conflicts   []depEntry   `xml:"rpm:conflicts>rpm:entry,omitempty"`
	Obsoletes   []depEntry   `xml:"rpm:obsoletes>rpm:entry,omitempty"`
}

type headerRange struct {
	Start int `xml:"start,attr"`
	End   int `xml:"end,attr"`
}

type depEntry struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr,omitempty"`
	Epoch string `xml:"epoch,attr,omitempty"`
	Ver   string `xml:"ver,attr,omitempty"`
	Rel   string `xml:"rel,attr,omitempty"`
	Pre   string `xml:"pre,attr,omitempty"`
}

type filelistsXML struct {
	XMLName  xml.Name           `xml:"filelists"`
	Xmlns    string             `xml:"xmlns,attr"`
	Count    int                `xml:"packages,attr"`
	Packages []filelistsPackage `xml:"package"`
}

type filelistsPackage struct {
	PkgID   string      `xml:"pkgid,attr"`
	Name    string      `xml:"name,attr"`
	Arch    string      `xml:"arch,attr"`
	Version rpmVersion  `xml:"version"`
	Files   []fileEntry `xml:"file"`
}

type fileEntry struct {
	Type string `xml:"type,attr,omitempty"`
	Path string `xml:",chardata"`
}

type otherXML struct {
	XMLName  xml.Name       `xml:"otherdata"`
	Xmlns    string         `xml:"xmlns,attr"`
	Count    int            `xml:"packages,attr"`
	Packages []otherPackage `xml:"package"`
}

type otherPackage struct {
	PkgID      string           `xml:"pkgid,attr"`
	Name       string           `xml:"name,attr"`
	Arch       string           `xml:"arch,attr"`
	Version    rpmVersion       `xml:"version"`
	Changelogs []changelogEntry `xml:"changelog"`
}

type changelogEntry struct {
	Author string `xml:"author,attr"`
	Date   int64  `xml:"date,attr"`
	Text   string `xml:",chardata"`
}

func parsePrimary(data []byte) (primaryXML, error) {
	var out primaryXML
	if err := xml.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func parseFilelistsInto(index map[string]*Package, data []byte) error {
	var fl filelistsXML
	if err := xml.Unmarshal(data, &fl); err != nil {
		return err
	}
	for _, p := range fl.Packages {
		pkg := index[p.PkgID]
		if pkg == nil {
			continue
		}
		for _, f := range p.Files {
			pkg.Files = append(pkg.Files, File{Path: f.Path, Type: f.Type})
		}
	}
	return nil
}

func parseOtherInto(index map[string]*Package, data []byte) error {
	var o otherXML
	if err := xml.Unmarshal(data, &o); err != nil {
		return err
	}
	for _, p := range o.Packages {
		pkg := index[p.PkgID]
		if pkg == nil {
			continue
		}
		for _, c := range p.Changelogs {
			pkg.Changelogs = append(pkg.Changelogs, Changelog{
				Author: c.Author,
				Date:   c.Date,
				Text:   c.Text,
			})
		}
	}
	return nil
}

func packageFromPrimary(p primaryPackage) Package {
	epoch := parseEpoch(p.Version.Epoch)
	headerStart, headerEnd := 0, 0
	if p.Format.HeaderRange != nil {
		headerStart = p.Format.HeaderRange.Start
		headerEnd = p.Format.HeaderRange.End
	}
	rel := Package{
		Name:          p.Name,
		Arch:          p.Arch,
		Epoch:         epoch,
		Version:       p.Version.Ver,
		Release:       p.Version.Rel,
		Summary:       p.Summary,
		Description:   p.Description,
		License:       p.Format.License,
		Vendor:        p.Format.Vendor,
		Group:         p.Format.Group,
		BuildHost:     p.Format.BuildHost,
		SourceRPM:     p.Format.SourceRPM,
		URL:           p.URL,
		Packager:      p.Packager,
		TimeBuild:     p.Time.Build,
		TimeFile:      p.Time.File,
		SizePackage:   p.Size.Package,
		SizeInstalled: p.Size.Installed,
		SizeArchive:   p.Size.Archive,
		Location:      p.Location.Href,
		PkgID:         p.Checksum.Value,
		ChecksumType:  p.Checksum.Type,
		HeaderStart:   headerStart,
		HeaderEnd:     headerEnd,
		Provides:      relationsFromEntries(p.Format.Provides),
		Requires:      relationsFromEntries(p.Format.Requires),
		Conflicts:     relationsFromEntries(p.Format.Conflicts),
		Obsoletes:     relationsFromEntries(p.Format.Obsoletes),
	}
	return rel
}

func marshalPrimary(pkgs []Package) ([]byte, error) {
	var out primaryXML
	out.Xmlns = CommonNamespace
	out.XmlnsRpm = RpmNamespace
	out.Count = len(pkgs)
	for _, p := range pkgs {
		pkg := primaryPackage{
			Type: "rpm",
			Name: p.Name,
			Arch: p.Arch,
			Version: rpmVersion{
				Epoch: strconv.Itoa(p.Epoch),
				Ver:   p.Version,
				Rel:   p.Release,
			},
			Checksum: rpmPkgChecksum{
				Type:  p.ChecksumType,
				PkgID: "YES",
				Value: p.PkgID,
			},
			Summary:     p.Summary,
			Description: p.Description,
			Packager:    p.Packager,
			URL:         p.URL,
			Time: primaryTime{
				File:  p.TimeFile,
				Build: p.TimeBuild,
			},
			Size: primarySize{
				Package:   p.SizePackage,
				Installed: p.SizeInstalled,
				Archive:   p.SizeArchive,
			},
			Location: Location{Href: p.Location},
			Format: primaryFormat{
				License:   p.License,
				Vendor:    p.Vendor,
				Group:     p.Group,
				BuildHost: p.BuildHost,
				SourceRPM: p.SourceRPM,
			},
		}
		if p.HeaderStart > 0 || p.HeaderEnd > 0 {
			pkg.Format.HeaderRange = &headerRange{Start: p.HeaderStart, End: p.HeaderEnd}
		}
		pkg.Format.Provides = entriesFromRelations(p.Provides)
		pkg.Format.Requires = entriesFromRelations(p.Requires)
		pkg.Format.Conflicts = entriesFromRelations(p.Conflicts)
		pkg.Format.Obsoletes = entriesFromRelations(p.Obsoletes)
		out.Packages = append(out.Packages, pkg)
	}
	return marshalWithHeader(out)
}

func marshalFilelists(pkgs []Package) ([]byte, error) {
	var out filelistsXML
	out.Xmlns = FilelistsNamespace
	out.Count = len(pkgs)
	for _, p := range pkgs {
		pkg := filelistsPackage{
			PkgID: p.PkgID,
			Name:  p.Name,
			Arch:  p.Arch,
			Version: rpmVersion{
				Epoch: strconv.Itoa(p.Epoch),
				Ver:   p.Version,
				Rel:   p.Release,
			},
		}
		for _, f := range p.Files {
			pkg.Files = append(pkg.Files, fileEntry{Type: f.Type, Path: f.Path})
		}
		out.Packages = append(out.Packages, pkg)
	}
	return marshalWithHeader(out)
}

func marshalOther(pkgs []Package) ([]byte, error) {
	var out otherXML
	out.Xmlns = OtherNamespace
	out.Count = len(pkgs)
	for _, p := range pkgs {
		pkg := otherPackage{
			PkgID: p.PkgID,
			Name:  p.Name,
			Arch:  p.Arch,
			Version: rpmVersion{
				Epoch: strconv.Itoa(p.Epoch),
				Ver:   p.Version,
				Rel:   p.Release,
			},
		}
		for _, c := range p.Changelogs {
			pkg.Changelogs = append(pkg.Changelogs, changelogEntry{
				Author: c.Author,
				Date:   c.Date,
				Text:   c.Text,
			})
		}
		out.Packages = append(out.Packages, pkg)
	}
	return marshalWithHeader(out)
}

func parseEpoch(s string) int {
	if s == "" {
		return 0
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

func relationsFromEntries(entries []depEntry) []Relation {
	var rels []Relation
	for _, e := range entries {
		r := Relation{
			Name:  e.Name,
			Flags: e.Flags,
			Epoch: parseEpoch(e.Epoch),
			Ver:   e.Ver,
			Rel:   e.Rel,
			Pre:   e.Pre == "1",
		}
		rels = append(rels, r)
	}
	return rels
}

func entriesFromRelations(rels []Relation) []depEntry {
	var entries []depEntry
	for _, r := range rels {
		e := depEntry{
			Name:  r.Name,
			Flags: r.Flags,
			Epoch: strconv.Itoa(r.Epoch),
			Ver:   r.Ver,
			Rel:   r.Rel,
		}
		if r.Pre {
			e.Pre = "1"
		}
		entries = append(entries, e)
	}
	return entries
}
