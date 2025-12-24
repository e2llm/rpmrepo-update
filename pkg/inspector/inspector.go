package inspector

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/cavaliergopher/rpm"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

// InspectRPM parses RPM data and builds metadata.Package describing it.
func InspectRPM(rpmPath string, rpmData []byte, info fs.FileInfo, checksumAlg, destRelPath string) (metadata.Package, error) {
	pkg, err := rpm.Read(bytes.NewReader(rpmData))
	if err != nil {
		return metadata.Package{}, fmt.Errorf("parse rpm %s: %w", rpmPath, err)
	}

	pkgID, err := metadata.ComputeChecksum(rpmData, checksumAlg)
	if err != nil {
		return metadata.Package{}, fmt.Errorf("checksum rpm %s: %w", rpmPath, err)
	}

	start, end := pkg.HeaderRange()
	infoSize := uint64(info.Size())
	buildTime := pkg.BuildTime().Unix()
	fileTime := info.ModTime().Unix()
	group := ""
	if g := pkg.Groups(); len(g) > 0 {
		group = g[0]
	}

	out := metadata.Package{
		Name:          pkg.Name(),
		Arch:          pkg.Architecture(),
		Epoch:         pkg.Epoch(),
		Version:       pkg.Version(),
		Release:       pkg.Release(),
		Summary:       pkg.Summary(),
		Description:   pkg.Description(),
		License:       pkg.License(),
		Vendor:        pkg.Vendor(),
		Group:         group,
		BuildHost:     pkg.BuildHost(),
		SourceRPM:     pkg.SourceRPM(),
		URL:           pkg.URL(),
		Packager:      pkg.Packager(),
		TimeBuild:     buildTime,
		TimeFile:      fileTime,
		SizePackage:   infoSize,
		SizeInstalled: pkg.Size(),
		SizeArchive:   pkg.ArchiveSize(),
		Location:      destRelPath,
		PkgID:         pkgID,
		ChecksumType:  checksumAlg,
		HeaderStart:   start,
		HeaderEnd:     end,
		Provides:      depsFromRPM(pkg.Provides()),
		Requires:      depsFromRPM(pkg.Requires()),
		Conflicts:     depsFromRPM(pkg.Conflicts()),
		Obsoletes:     depsFromRPM(pkg.Obsoletes()),
	}

	out.Files = filesFromRPM(pkg.Files())
	out.Changelogs = changelogsFromRPM(pkg)
	return out, nil
}

func depsFromRPM(deps []rpm.Dependency) []metadata.Relation {
	var out []metadata.Relation
	for _, d := range deps {
		flags, pre := depFlagsToString(d.Flags())
		out = append(out, metadata.Relation{
			Name:  d.Name(),
			Flags: flags,
			Epoch: d.Epoch(),
			Ver:   d.Version(),
			Rel:   d.Release(),
			Pre:   pre,
		})
	}
	return out
}

func filesFromRPM(files []rpm.FileInfo) []metadata.File {
	var out []metadata.File
	for _, f := range files {
		ftype := ""
		if f.Flags()&rpm.FileFlagGhost != 0 {
			ftype = "ghost"
		} else if f.IsDir() {
			ftype = "dir"
		}
		out = append(out, metadata.File{
			Path: f.Name(),
			Type: ftype,
		})
	}
	return out
}

func changelogsFromRPM(pkg *rpm.Package) []metadata.Changelog {
	// Changelog fields use tag IDs 1080/1081/1082.
	times := pkg.Header.GetTag(1080).Int64Slice()
	names := pkg.Header.GetTag(1081).StringSlice()
	texts := pkg.Header.GetTag(1082).StringSlice()
	n := min(len(times), len(names), len(texts))
	entries := make([]metadata.Changelog, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, metadata.Changelog{
			Author: names[i],
			Date:   times[i],
			Text:   texts[i],
		})
	}
	return entries
}

func depFlagsToString(flags int) (string, bool) {
	pre := flags&rpm.DepFlagPrereq != 0
	switch {
	case flags&rpm.DepFlagLesserOrEqual == rpm.DepFlagLesserOrEqual:
		return "LE", pre
	case flags&rpm.DepFlagGreaterOrEqual == rpm.DepFlagGreaterOrEqual:
		return "GE", pre
	case flags&rpm.DepFlagLesser == rpm.DepFlagLesser:
		return "LT", pre
	case flags&rpm.DepFlagGreater == rpm.DepFlagGreater:
		return "GT", pre
	case flags&rpm.DepFlagEqual == rpm.DepFlagEqual:
		return "EQ", pre
	default:
		return "", pre
	}
}

func min(vals ...int) int {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
