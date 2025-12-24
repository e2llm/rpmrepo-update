package metadata

import (
	"encoding/xml"
)

const RepoNamespace = "http://linux.duke.edu/metadata/repo"

type RepoMD struct {
	XMLName  xml.Name   `xml:"repomd"`
	Xmlns    string     `xml:"xmlns,attr"`
	Revision string     `xml:"revision"`
	Data     []RepoData `xml:"data"`
}

type RepoData struct {
	Type         string    `xml:"type,attr"`
	Checksum     Checksum  `xml:"checksum"`
	OpenChecksum *Checksum `xml:"open-checksum,omitempty"`
	Location     Location  `xml:"location"`
	Timestamp    int64     `xml:"timestamp"`
	Size         int64     `xml:"size"`
	OpenSize     int64     `xml:"open-size"`
}

type Checksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type Location struct {
	Href string `xml:"href,attr"`
}

func MarshalRepoMD(md RepoMD) ([]byte, error) {
	if md.Xmlns == "" {
		md.Xmlns = RepoNamespace
	}
	output, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), output...), nil
}
