package files

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"remvyn/backend/ids"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

func localExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// ReadTextLocal opens a local text file for editing.
func ReadTextLocal(p string) (models.FileContent, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return models.FileContent{}, err
	}
	if err := checkEditable(fi, p); err != nil {
		return models.FileContent{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return models.FileContent{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxEditSize+1))
	if err != nil {
		return models.FileContent{}, err
	}
	return textContent(p, data, fi)
}

// WriteTextLocal saves a local text file, failing with CodeFileChanged if
// expectModTime is non-zero and the file changed since then.
func WriteTextLocal(p, content string, expectModTime int64) (models.FileContent, error) {
	// Write through symlinks to the file they point at.
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	mode := fs.FileMode(0o644)
	fi, err := os.Stat(p)
	switch {
	case err == nil:
		if fi.IsDir() {
			return models.FileContent{}, fmt.Errorf("%s is a folder", p)
		}
		if expectModTime != 0 && fi.ModTime().UnixMilli() != expectModTime {
			return models.FileContent{}, sshconn.Coded(CodeFileChanged, filepath.Base(p)+" was changed after you opened it", nil)
		}
		mode = fi.Mode().Perm()
	case !errors.Is(err, os.ErrNotExist):
		return models.FileContent{}, err
	}
	if err := writeFileAtomic(p, []byte(content), mode); err != nil {
		return models.FileContent{}, err
	}
	nfi, err := os.Stat(p)
	if err != nil {
		return models.FileContent{}, err
	}
	return models.FileContent{Path: p, Size: nfi.Size(), ModTime: nfi.ModTime().UnixMilli()}, nil
}

func writeFileAtomic(p string, data []byte, mode fs.FileMode) error {
	tmp := filepath.Join(filepath.Dir(p), "."+filepath.Base(p)+".remvyn-"+ids.New()[:8])
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		// No write access to the folder: overwrite in place.
		return os.WriteFile(p, data, mode)
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, p)
	}
	if werr != nil {
		os.Remove(tmp)
	}
	return werr
}

// CreateFileLocal makes an empty local file inside dir and returns its path.
func CreateFileLocal(dir, name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if localExists(p) {
			return "", fmt.Errorf("%s already exists", name)
		}
		return "", err
	}
	return p, f.Close()
}

// within reports whether p is dir or inside it.
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// CopyLocal copies local entries into dir. Copying into the same folder
// adds " copy" to the name.
func CopyLocal(paths []string, dir string) error {
	dir = filepath.Clean(dir)
	for _, p := range paths {
		p = filepath.Clean(p)
		if within(p, dir) {
			return fmt.Errorf("can't copy %s into itself", filepath.Base(p))
		}
		dst := filepath.Join(dir, freeName(localExists, filepath.Join, dir, filepath.Base(p)))
		if err := copyLocalTree(p, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyLocalTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		fi, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, fi.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case !d.Type().IsRegular():
			return nil
		}
		return copyLocalFile(p, target, fi.Mode().Perm())
	})
}

func copyLocalFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// MoveLocal moves local entries into dir.
func MoveLocal(paths []string, dir string) error {
	dir = filepath.Clean(dir)
	for _, p := range paths {
		p = filepath.Clean(p)
		if filepath.Dir(p) == dir {
			continue
		}
		if within(p, dir) {
			return fmt.Errorf("can't move %s into itself", filepath.Base(p))
		}
		dst := filepath.Join(dir, filepath.Base(p))
		if localExists(dst) {
			return fmt.Errorf("%s already exists in %s", filepath.Base(p), dir)
		}
		if err := os.Rename(p, dst); err != nil {
			// Different drive or filesystem: copy, then delete the original.
			if cerr := copyLocalTree(p, dst); cerr != nil {
				os.RemoveAll(dst)
				return cerr
			}
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// CompressLocal archives entries of dir into dir/archiveName and returns
// the archive's path.
func CompressLocal(dir string, names []string, archiveName, format string) (string, error) {
	if err := validName(archiveName); err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", errors.New("nothing to compress")
	}
	for _, n := range names {
		if err := validName(n); err != nil {
			return "", err
		}
	}
	out := filepath.Join(dir, archiveName)
	if localExists(out) {
		return "", fmt.Errorf("%s already exists", archiveName)
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	switch format {
	case FormatZip:
		err = writeZip(f, dir, names)
	case FormatTarGz:
		err = writeTarGz(f, dir, names)
	default:
		err = fmt.Errorf("unknown archive format %q", format)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out)
		return "", err
	}
	return out, nil
}

// walkNames visits every file and folder under dir/names, skipping out (the
// archive being written) and symlinked folders.
func walkNames(dir string, names []string, out string, visit func(p, rel string, fi fs.FileInfo) error) error {
	for _, n := range names {
		root := filepath.Join(dir, n)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == out {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				if fi, err = os.Stat(p); err != nil || fi.IsDir() {
					return nil
				}
			}
			rel, _ := filepath.Rel(dir, p)
			return visit(p, filepath.ToSlash(rel), fi)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func writeZip(w io.Writer, dir string, names []string) error {
	zw := zip.NewWriter(w)
	out := ""
	if f, ok := w.(*os.File); ok {
		out = f.Name()
	}
	err := walkNames(dir, names, out, func(p, rel string, fi fs.FileInfo) error {
		h, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		h.Name = rel
		if fi.IsDir() {
			h.Name += "/"
			_, err = zw.CreateHeader(h)
			return err
		}
		h.Method = zip.Deflate
		dst, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		return copyFrom(dst, p)
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	return err
}

func writeTarGz(w io.Writer, dir string, names []string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	out := ""
	if f, ok := w.(*os.File); ok {
		out = f.Name()
	}
	err := walkNames(dir, names, out, func(p, rel string, fi fs.FileInfo) error {
		h, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		h.Name = rel
		if fi.IsDir() {
			h.Name += "/"
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		return copyFrom(tw, p)
	})
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	return err
}

func copyFrom(w io.Writer, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// ExtractLocal unpacks a local archive into its folder, or into a new
// folder named after it when intoFolder is set. Entries that would land
// outside the destination ("zip slip") are rejected.
func ExtractLocal(archive string, intoFolder bool) (string, error) {
	archive = filepath.Clean(archive)
	name := filepath.Base(archive)
	format := archiveFormat(name)
	if format == "" {
		return "", fmt.Errorf("%s is not an archive Remvyn can extract", name)
	}
	if format == ".tar.xz" || format == ".txz" {
		return "", errors.New(".tar.xz archives can only be extracted on a server")
	}
	dest := filepath.Dir(archive)
	if intoFolder {
		dest = filepath.Join(dest, freeName(localExists, filepath.Join, dest, archiveStem(name)))
		if err := os.Mkdir(dest, 0o755); err != nil {
			return "", err
		}
	}
	var err error
	switch format {
	case ".zip", ".jar", ".war":
		err = extractZip(archive, dest)
	case ".gz":
		err = extractGz(archive, filepath.Join(dest, freeName(localExists, filepath.Join, dest, archiveStem(name))))
	default:
		err = extractTar(archive, dest, format)
	}
	if err != nil && intoFolder {
		os.Remove(dest) // only succeeds if nothing was extracted
	}
	return dest, err
}

// safeTarget joins an archive entry name onto dest, rejecting escapes.
func safeTarget(dest, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("refusing to extract %q: unsafe path", name)
	}
	target := filepath.Join(dest, filepath.FromSlash(name))
	if !within(dest, target) {
		return "", fmt.Errorf("refusing to extract %q: it would be written outside the folder", name)
	}
	return target, nil
}

func extractZip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		target, err := safeTarget(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			continue // skip symlinks and devices
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		err = writeExtracted(target, src, f.Mode().Perm())
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(archive, dest, format string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	switch format {
	case ".tar.gz", ".tgz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	case ".tar.bz2", ".tbz2":
		r = bzip2.NewReader(f)
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeTarget(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeExtracted(target, tr, fs.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		}
		// Links and special files are skipped.
	}
}

func extractGz(archive, target string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	return writeExtracted(target, gz, 0o644)
}

func writeExtracted(target string, r io.Reader, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, r)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
