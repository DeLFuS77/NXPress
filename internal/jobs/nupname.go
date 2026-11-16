package jobs

// NupNameBy is the name of the NUP of a dump folder by a template, the one of the name of each
// SuperNSP in Combine: its fields are the ones of any name ({name} {id} {version} {version_txt}
// {type} {titles} {contents} {size_gb} {size_mb} ...), of the main title of the folder and what it
// holds. With no template it is NupName: the name of the game, then its title ID and newest
// version.
func NupNameBy(dir string, template string) (string, error) {
	if template == "" {
		return NupName(dir)
	}
	title, err := dumpTitle(dir)
	if err != nil {
		return "", err
	}
	// the extension is the one of a NUP whatever the folder is called (a dot in its name)
	return NewName(dir+".nup", title, template), nil
}
