package optimize

// RebuildResult is what Rebuild did to a package.
type RebuildResult struct {
	Titles     int   // titles (CNMT) in it
	NCAs       int   // NCA files besides the meta NCAs
	Renamed    int   // NCA files whose content ID (name) changed
	Standard   int   // NCA files moved from a rights ID and a ticket to standard crypto
	FsFixed    int   // FS headers whose hash in the NCA header was wrong, made right
	Dropped    int   // files left out: tickets, certificates, delta fragments and other files that are not NCA
	Download   int   // NCA files marked for a game card, marked as downloaded
	InputSize  int64 // the package given, as an NSP
	OutputSize int64 // the new NSP
}
