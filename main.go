package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"os/exec"

	"github.com/dietsche/rfsnotify"
	"github.com/thoas/go-funk"
	"gopkg.in/fsnotify.v1"
)

type SHASUMItem struct {
	Hash     string
	FileName string
}

func main() {
	// If exists commands
	log.Println("必要なコマンドの存在確認を行います...")
	checkExistsCommand("git")
	log.Println("必要なコマンドの存在確認が完了しました。")

	log.Println(".gitignoreへの除外挿入を行います...")
	addGitIgnore("jaoweb-viewer*")
	addGitIgnore("node/")
	addGitIgnore("jaoweb/")
	log.Println(".gitignoreへの除外挿入を行いました。")

	log.Println("node.jsの既ダウンロード済み検索、もしくはダウンロードを行います...")
	nodepath := downloadNodeJS()
	log.Println("node.jsの既ダウンロード済み検索、もしくはダウンロードを行いました")
	log.Println("node.jsのパス: ", nodepath)

	execCommandStdout(nodepath, "--version").Run()

	if _, err := os.Stat("jaoweb"); os.IsNotExist(err) {
		log.Println("jaowebが未ダウンロードのため、git cloneによるダウンロードを行います")
		execCommandStdout("git", "clone", "https://github.com/jaoafa/jaoweb").Run()
		log.Println("jaowebのダウンロードを行いました。")
	} else {
		log.Println("jaowebがダウンロード済みのため、git pullによる更新を行います")
		os.Chdir("jaoweb/")
		gitpull := execCommandStdout("git", "pull")
		gitpull.Stderr = os.Stderr
		gitpull.Stdout = os.Stdout
		gitpull.Stdin = os.Stdin
		gitpull.Run()
		os.Chdir("../")
		log.Println("jaowebの更新を行いました。")
	}

	log.Println("カレントディレクトリをjaowebに変更します。")
	os.Chdir("jaoweb/")
	if _, err := os.Stat("content/.git"); os.IsNotExist(err) {
		if _, err2 := os.Stat("content"); !os.IsNotExist(err2) {
			log.Println("contentが既に存在するため、削除します。")
			e := os.RemoveAll("content")
			if e != nil {
				log.Fatal(e)
			}
		}
		log.Println("jaoweb-docsが未ダウンロードのため、git cloneによるダウンロードを行います。")
		log.Println("フォーク先のリポジトリをクローンするため、あなたのGitHubアカウント名を入力してください。（事前にフォークする必要があります）")
		log.Print("あなたのGitHubアカウント名: ")
		var owner string
		fmt.Scan(&owner)

		log.Println(owner + "/jaoweb-docs からクローンします。")
		execCommandStdout("git", "clone", "https://github.com/"+owner+"/jaoweb-docs", "content").Run()
		log.Println("upstreamリモートを追加します。")
		execCommandStdout("git", "remote", "add", "upstream", "https://github.com/jaoafa/jaoweb-docs")
		log.Println("jaowebのダウンロードを行いました。")
	}
	log.Println("カレントディレクトリを戻します。")
	os.Chdir("../")

	var npxpath string
	switch runtime.GOOS {
	case "linux":
		log.Println("npxを探します...")
		npxpath = searchFile("npx")
		log.Println("npxを探しました。")
		log.Println("npxのパス: ", npxpath)
	case "windows":
		log.Println("npx.cmdを探します...")
		npxpath = searchFile("npx.cmd")
		log.Println("npx.cmdを探しました。")
		log.Println("npx.cmdのパス: ", npxpath)
	case "darwin":
		log.Println("npxを探します...")
		npxpath = searchFile("npx")
		log.Println("npxを探しました。")
		log.Println("npxのパス: ", npxpath)
	default:
		log.Fatal("unsupported platform")
	}

	log.Println("カレントディレクトリをjaowebに変更します。")
	os.Chdir("jaoweb/")

	log.Println("依存パッケージをダウンロードします。")
	err := execCommandStdout(npxpath, "yarn", "install").Run()
	if err != nil {
		log.Fatal(err)
		return
	}
	log.Println("依存パッケージをダウンロードしました。")

	log.Println("開発サーバを起動します。")
	openbrowser("http://localhost:3000")
	yarndev := execCommandStdout(npxpath, "yarn", "dev")
	yarndev.Env = append(os.Environ(), "NUXT_TELEMETRY_DISABLED=1")
	yarndev.Start()
	defer yarndev.Process.Kill() // Ctrl+Cの場合動作しない？

	if isExistsCommand("code") {
		cwd, _ := os.Getwd()
		execCommandStdout("code", filepath.Join(cwd, "content")).Run()
	}

	watcher, err := rfsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					abspath, _ := filepath.Abs(event.Name)
					topath, _ := filepath.Abs(filepath.Join("content", strings.TrimPrefix(event.Name, "..")))
					stat, _ := os.Stat(abspath)
					if stat.IsDir() {
						continue
					}
					todir := filepath.Dir(topath)
					log.Println("ファイルが変更されました。更新します:", strings.TrimPrefix(event.Name, ".."))

					if _, err := os.Stat(todir); os.IsNotExist(err) {
						os.Mkdir(todir, 0644)
					}

					err := copyFile(abspath, topath)
					if err != nil {
						panic(err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	watcher.AddRecursive("..")
	watcher.RemoveRecursive(".")
	watcher.RemoveRecursive("../jaoweb")
	watcher.RemoveRecursive("../.git")
	<-done
}

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = execCommandStdout("xdg-open", url).Start()
	case "windows":
		err = execCommandStdout("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = execCommandStdout("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}

}

func execCommandStdout(name string, arg ...string) *exec.Cmd {
	command := exec.Command(name, arg...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command
}

func addGitIgnore(str string) {
	bytes, err := ioutil.ReadFile(".gitignore")
	if err != nil {
		ioutil.WriteFile(".gitignore", []byte(""), os.ModePerm)
	}
	if strings.Contains(string(bytes), str) {
		return
	}

	ioutil.WriteFile(".gitignore", []byte(string(bytes)+"\n"+str), os.ModePerm)
}

func checkExistsCommand(command string) {
	result := isExistsCommand(command)
	if !result {
		log.Fatal("コマンドが見つかりませんでした:", command)
		log.Fatal("必要なアプリケーションなどがインストールされているかどうか確認してください。")
		os.Exit(1)
	}
}

func isExistsCommand(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func parseSHASUM(url string) []SHASUMItem {
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	var items []SHASUMItem

	buf := bytes.NewBufferString(string(body))
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), "  ")
		items = append(items, SHASUMItem{strings.TrimSpace(line[0]), strings.TrimSpace(line[1])})
	}

	return items
}

func getOSNodeSuffix() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return "x64.tar.gz", nil
	case "windows":
		return "win-x64.zip", nil
	case "darwin":
		return "darwin-x64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported platform")
	}
}

func downloadNodeJS() string {
	if _, err := os.Stat("node"); os.IsNotExist(err) {
		os.Mkdir("node", 0644)
	}

	winnodepath := searchFile("node.exe")
	if winnodepath != "" {
		return winnodepath
	}

	nodepath := searchFile("node")
	if nodepath != "" {
		return nodepath
	}

	suffix, err := getOSNodeSuffix()
	if err != nil {
		log.Fatal(err)
	}

	zippath := "node/node-" + suffix

	items := parseSHASUM("https://nodejs.org/dist/latest-v14.x/SHASUMS256.txt")
	var matched SHASUMItem = funk.Find(items, func(item SHASUMItem) bool {
		return strings.HasSuffix(item.FileName, suffix)
	}).(SHASUMItem)

	log.Println("node-"+suffix+"をダウンロードします:", "https://nodejs.org/dist/latest-v14.x/"+matched.FileName)
	downloadFile(zippath, "https://nodejs.org/dist/latest-v14.x/"+matched.FileName)

	distpath := "node/"
	extract(zippath, distpath)

	nodepath = searchFile("node.exe")
	if nodepath != "" {
		return nodepath
	}

	return ""
}

func searchFile(filename string) string {
	var path string
	filepath.Walk(".", func(ret *string) filepath.WalkFunc {
		return func(path string, f os.FileInfo, err error) error {
			if f.IsDir() {
				return nil
			}
			if strings.HasSuffix(f.Name(), filename) {
				*ret = path
			}
			return nil
		}
	}(&path))
	if path != "" {
		path, _ = filepath.Abs(path)
	}
	return path
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extract(filepath string, distpath string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = gunzip(filepath, distpath)
	case "windows":
		err = unzip(filepath, distpath)
	case "darwin":
		err = gunzip(filepath, distpath)
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		if f.FileInfo().IsDir() {
			path := filepath.Join(dest, f.Name)
			os.MkdirAll(path, f.Mode())
		} else {
			buf := make([]byte, f.UncompressedSize)
			_, err = io.ReadFull(rc, buf)
			if err != nil {
				return err
			}

			path := filepath.Join(dest, f.Name)
			if err = ioutil.WriteFile(path, buf, f.Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}

func gunzip(src, dest string) error {
	gzipStream, err := os.Open(src)
	if err != nil {
		return err
	}

	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			path := filepath.Join(dest, header.Name)
			if err := os.Mkdir(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			path := filepath.Join(dest, header.Name)
			outFile, err := os.Create(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}
			outFile.Close()

		default:
			return fmt.Errorf(
				"ExtractTarGz: uknown type: %s in %s",
				string(header.Typeflag),
				header.Name)
		}
	}
	return nil
}

func copyFile(src string, dest string) error {
	b, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	// write the whole body at once
	err = ioutil.WriteFile(dest, b, 0644)
	if err != nil {
		return err
	}
	return nil
}
