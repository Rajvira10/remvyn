package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"

	"remvyn/backend/ids"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// MaxEditSize is the largest file the built-in editor opens.
const MaxEditSize = 5 << 20

// Error codes for editor and archive operations.
const (
	CodeBinary      = "binary_file"
	CodeTooLarge    = "file_too_large"
	CodeFileChanged = "file_changed"
)

// Archive formats for Compress.
const (
	FormatZip   = "zip"
	FormatTarGz = "tar.gz"
)

// remoteCmdTimeout bounds server-side copies, compression and extraction.
const remoteCmdTimeout = 30 * time.Minute

func checkEditable(fi fs.FileInfo, p string) error {
	if fi.IsDir() {
		return fmt.Errorf("%s is a folder", p)
	}
	if fi.Size() > MaxEditSize {
		return sshconn.Coded(CodeTooLarge, fmt.Sprintf("%s is too large to edit in Remvyn (%d MB; the limit is 5 MB)", path.Base(p), fi.Size()>>20), nil)
	}
	return nil
}

func textContent(p string, data []byte, fi fs.FileInfo) (models.FileContent, error) {
	if len(data) > MaxEditSize {
		return models.FileContent{}, sshconn.Coded(CodeTooLarge, path.Base(p)+" is too large to edit in Remvyn (the limit is 5 MB)", nil)
	}
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(data) {
		return models.FileContent{}, sshconn.Coded(CodeBinary, path.Base(p)+" is not a text file", nil)
	}
	return models.FileContent{Path: p, Content: string(data), Size: int64(len(data)), ModTime: fi.ModTime().UnixMilli()}, nil
}

// ReadText opens a remote text file for editing.
func (m *Manager) ReadText(id, p string) (models.FileContent, error) {
	s, err := m.get(id)
	if err != nil {
		return models.FileContent{}, err
	}
	fi, err := s.sftp.Stat(p)
	if err != nil {
		return models.FileContent{}, remoteErr(err, p)
	}
	if err := checkEditable(fi, p); err != nil {
		return models.FileContent{}, err
	}
	f, err := s.sftp.Open(p)
	if err != nil {
		return models.FileContent{}, remoteErr(err, p)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxEditSize+1))
	if err != nil {
		return models.FileContent{}, remoteErr(err, p)
	}
	return textContent(p, data, fi)
}

// WriteText saves a remote text file. If expectModTime is non-zero and the
// file has changed since then, it fails with CodeFileChanged.
func (m *Manager) WriteText(id, p, content string, expectModTime int64) (models.FileContent, error) {
	s, err := m.get(id)
	if err != nil {
		return models.FileContent{}, err
	}
	fi, err := s.sftp.Stat(p)
	switch {
	case err == nil:
		if fi.IsDir() {
			return models.FileContent{}, fmt.Errorf("%s is a folder", p)
		}
		if expectModTime != 0 && fi.ModTime().UnixMilli() != expectModTime {
			return models.FileContent{}, sshconn.Coded(CodeFileChanged, path.Base(p)+" was changed on the server after you opened it", nil)
		}
	case errors.Is(err, os.ErrNotExist):
		fi = nil
	default:
		return models.FileContent{}, remoteErr(err, p)
	}
	if err := writeRemote(s.sftp, p, []byte(content), fi); err != nil {
		return models.FileContent{}, remoteErr(err, p)
	}
	nfi, err := s.sftp.Stat(p)
	if err != nil {
		return models.FileContent{}, remoteErr(err, p)
	}
	return models.FileContent{Path: p, Size: nfi.Size(), ModTime: nfi.ModTime().UnixMilli()}, nil
}

// writeRemote replaces p with data. It writes a temp file and renames it
// over p so a dropped connection never leaves a truncated file, keeping the
// mode and owner. When that isn't possible (no write access to the folder,
// a symlink, another owner, no posix-rename) it overwrites p in place.
func writeRemote(c *sftp.Client, p string, data []byte, fi fs.FileInfo) error {
	if fi != nil {
		if lfi, err := c.Lstat(p); err == nil && lfi.Mode()&fs.ModeSymlink != 0 {
			return writeRemoteInPlace(c, p, data)
		}
	}
	mode := fs.FileMode(0o644)
	if fi != nil {
		mode = fi.Mode().Perm()
	}
	tmp := path.Join(path.Dir(p), "."+path.Base(p)+".remvyn-"+ids.New()[:8])
	f, err := c.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return writeRemoteInPlace(c, p, data)
	}
	// Set the mode before writing so a private file is never readable.
	_ = f.Chmod(mode)
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = c.Remove(tmp)
		return werr
	}
	if fi != nil && !sameOwner(c, tmp, fi) {
		_ = c.Remove(tmp)
		return writeRemoteInPlace(c, p, data)
	}
	if err := c.PosixRename(tmp, p); err != nil {
		_ = c.Remove(tmp)
		return writeRemoteInPlace(c, p, data)
	}
	return nil
}

// sameOwner gives tmp the owner of orig if it differs, and reports whether
// they now match.
func sameOwner(c *sftp.Client, tmp string, orig fs.FileInfo) bool {
	want, ok := orig.Sys().(*sftp.FileStat)
	if !ok {
		return true
	}
	tfi, err := c.Stat(tmp)
	if err != nil {
		return false
	}
	have, ok := tfi.Sys().(*sftp.FileStat)
	if !ok || (have.UID == want.UID && have.GID == want.GID) {
		return true
	}
	return c.Chown(tmp, int(want.UID), int(want.GID)) == nil
}

func writeRemoteInPlace(c *sftp.Client, p string, data []byte) error {
	f, err := c.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// CreateFile makes an empty remote file inside dir and returns its path.
func (m *Manager) CreateFile(id, dir, name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	s, err := m.get(id)
	if err != nil {
		return "", err
	}
	p := path.Join(dir, name)
	f, err := s.sftp.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		if _, serr := s.sftp.Lstat(p); serr == nil {
			return "", fmt.Errorf("%s already exists", name)
		}
		return "", remoteErr(err, p)
	}
	return p, f.Close()
}

// Chmod sets the permission bits of a remote entry.
func (m *Manager) Chmod(id, p string, mode uint32) error {
	if mode > 0o7777 {
		return fmt.Errorf("invalid mode %o", mode)
	}
	s, err := m.get(id)
	if err != nil {
		return err
	}
	return remoteErr(s.sftp.Chmod(p, fs.FileMode(mode&0o777)|unixSpecial(mode)), p)
}

// unixSpecial maps setuid/setgid/sticky octal bits to fs.FileMode.
func unixSpecial(mode uint32) fs.FileMode {
	var m fs.FileMode
	if mode&0o4000 != 0 {
		m |= fs.ModeSetuid
	}
	if mode&0o2000 != 0 {
		m |= fs.ModeSetgid
	}
	if mode&0o1000 != 0 {
		m |= fs.ModeSticky
	}
	return m
}

// run executes a shell command on the server, returning its output as the
// error message if it fails.
func (s *sftpSession) run(ctx context.Context, script string) error {
	sess, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("could not run a command on the server: %w", err)
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &out
	done := make(chan error, 1)
	go func() { done <- sess.Run(script) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		return errors.New("the command on the server took too long")
	}
	if err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			msg = err.Error()
		}
		if lines := strings.Split(msg, "\n"); len(lines) > 4 {
			msg = strings.Join(lines[len(lines)-4:], "\n")
		}
		return errors.New(msg)
	}
	return nil
}

// shq quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// freeName returns a name in dir based on name that exists nowhere yet:
// "name", then "name copy", "name copy 2", … (before the extension).
func freeName(exists func(string) bool, join func(...string) string, dir, name string) string {
	if !exists(join(dir, name)) {
		return name
	}
	stem, ext := splitExt(name)
	for i := 1; ; i++ {
		cand := stem + " copy" + ext
		if i > 1 {
			cand = fmt.Sprintf("%s copy %d%s", stem, i, ext)
		}
		if !exists(join(dir, cand)) {
			return cand
		}
	}
}

// splitExt splits "a.tar.gz" into "a", ".tar.gz" and "a.txt" into "a", ".txt".
// Hidden files without another dot keep their whole name as the stem.
func splitExt(name string) (string, string) {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz"} {
		if strings.HasSuffix(lower, ext) && len(name) > len(ext) {
			return name[:len(name)-len(ext)], name[len(name)-len(ext):]
		}
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i], name[i:]
	}
	return name, ""
}

func (s *sftpSession) exists(p string) bool {
	_, err := s.sftp.Lstat(p)
	return err == nil
}

// Copy copies remote entries into dir on the server. Copying into the
// same folder adds " copy" to the name.
func (m *Manager) Copy(id string, paths []string, dir string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteCmdTimeout)
	defer cancel()
	dir = path.Clean(dir)
	for _, p := range paths {
		p = path.Clean(p)
		if dir == p || strings.HasPrefix(dir, p+"/") {
			return fmt.Errorf("can't copy %s into itself", path.Base(p))
		}
		dst := path.Join(dir, freeName(s.exists, path.Join, dir, path.Base(p)))
		if err := s.run(ctx, copyScript(p, dst)); err != nil {
			return err
		}
	}
	return nil
}

// Move moves remote entries into dir.
func (m *Manager) Move(id string, paths []string, dir string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	dir = path.Clean(dir)
	for _, p := range paths {
		p = path.Clean(p)
		if path.Dir(p) == dir {
			continue
		}
		if dir == p || strings.HasPrefix(dir, p+"/") {
			return fmt.Errorf("can't move %s into itself", path.Base(p))
		}
		dst := path.Join(dir, path.Base(p))
		if s.exists(dst) {
			return fmt.Errorf("%s already exists in %s", path.Base(p), dir)
		}
		if err := s.sftp.Rename(p, dst); err != nil {
			return remoteErr(err, p)
		}
	}
	return nil
}

// Compress archives entries of dir into dir/archiveName on the server and
// returns the archive's path. format is FormatZip or FormatTarGz.
func (m *Manager) Compress(id, dir string, names []string, archiveName, format string) (string, error) {
	if err := validName(archiveName); err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", errors.New("nothing to compress")
	}
	s, err := m.get(id)
	if err != nil {
		return "", err
	}
	out := path.Join(dir, archiveName)
	if s.exists(out) {
		return "", fmt.Errorf("%s already exists", archiveName)
	}
	script, err := compressScript(dir, names, archiveName, format)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteCmdTimeout)
	defer cancel()
	if err := s.run(ctx, script); err != nil {
		return "", err
	}
	return out, nil
}

// compressScript is the shell command that archives names (entries of dir)
// into dir/archiveName.
func compressScript(dir string, names []string, archiveName, format string) (string, error) {
	var items []string
	for _, n := range names {
		if err := validName(n); err != nil {
			return "", err
		}
		// "./name" so names starting with "-" aren't read as options.
		items = append(items, shq("./"+n))
	}
	list := strings.Join(items, " ")
	switch format {
	case FormatZip:
		return fmt.Sprintf("cd %s && if command -v zip >/dev/null 2>&1; then zip -q -r -y %s %s; "+
			"else echo 'zip is not installed on the server. Try .tar.gz instead.' >&2; exit 127; fi", shq(dir), shq(archiveName), list), nil
	case FormatTarGz:
		return fmt.Sprintf("cd %s && tar -czf %s %s", shq(dir), shq(archiveName), list), nil
	}
	return "", fmt.Errorf("unknown archive format %q", format)
}

// archiveFormat names the archive type of a file, or "" if it isn't one.
func archiveFormat(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".tar", ".zip", ".jar", ".war", ".gz"} {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return ""
}

// archiveStem is the archive name without its extension.
func archiveStem(name string) string {
	if ext := archiveFormat(name); ext != "" && len(name) > len(ext) {
		return name[:len(name)-len(ext)]
	}
	return name
}

// Extract unpacks a remote archive into its folder, or into a new folder
// named after it when intoFolder is set. It returns the destination folder.
func (m *Manager) Extract(id, archive string, intoFolder bool) (string, error) {
	s, err := m.get(id)
	if err != nil {
		return "", err
	}
	archive = path.Clean(archive)
	name := path.Base(archive)
	format := archiveFormat(name)
	if format == "" {
		return "", fmt.Errorf("%s is not an archive Remvyn can extract", name)
	}
	dest := path.Dir(archive)
	if intoFolder {
		dest = path.Join(dest, freeName(s.exists, path.Join, dest, archiveStem(name)))
		if err := s.sftp.Mkdir(dest); err != nil {
			return "", remoteErr(err, dest)
		}
	}
	gzOut := ""
	if format == ".gz" {
		gzOut = freeName(s.exists, path.Join, dest, archiveStem(name))
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteCmdTimeout)
	defer cancel()
	if err := s.run(ctx, extractScript(archive, dest, gzOut)); err != nil {
		if intoFolder {
			_ = s.sftp.RemoveDirectory(dest) // only succeeds if nothing was extracted
		}
		return "", err
	}
	return dest, nil
}

// extractScript is the shell command that unpacks archive into dest. A
// plain .gz file is decompressed to dest/gzOut.
func extractScript(archive, dest, gzOut string) string {
	a := shq(archive)
	var cmd string
	switch archiveFormat(path.Base(archive)) {
	case ".zip", ".jar", ".war":
		cmd = "if command -v unzip >/dev/null 2>&1; then unzip -o -q " + a + "; " +
			"elif command -v bsdtar >/dev/null 2>&1; then bsdtar -xf " + a + "; " +
			"elif command -v python3 >/dev/null 2>&1; then python3 -m zipfile -e " + a + " .; " +
			"else echo 'unzip is not installed on the server' >&2; exit 127; fi"
	case ".tar.gz", ".tgz":
		cmd = "tar -xzf " + a
	case ".tar.bz2", ".tbz2":
		cmd = "tar -xjf " + a
	case ".tar.xz", ".txz":
		cmd = "tar -xJf " + a
	case ".tar":
		cmd = "tar -xf " + a
	case ".gz":
		cmd = "gzip -dc " + a + " > " + shq(gzOut)
	}
	return "cd " + shq(dest) + " && " + cmd
}

// copyScript is the shell command that copies src to dst, keeping modes,
// times and symlinks.
func copyScript(src, dst string) string { return "cp -pR -- " + shq(src) + " " + shq(dst) }
