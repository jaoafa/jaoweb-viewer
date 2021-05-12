package main

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"os/exec"

	"github.com/dietsche/rfsnotify"
	"gopkg.in/fsnotify.v1"
)

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

	if _, err := os.Stat("jaoweb"); os.IsNotExist(err) {
		log.Println("jaowebが未ダウンロードのため、git cloneによるダウンロードを行います")
		exec.Command("git", "clone", "https://github.com/jaoafa/jaoweb").Run()
		log.Println("jaowebのダウンロードを行いました。")
	} else {
		log.Println("jaowebがダウンロード済みのため、git pullによる更新を行います")
		os.Chdir("jaoweb/")
		exec.Command("git", "pull").Run()
		os.Chdir("../")
		log.Println("jaowebの更新を行いました。")
	}

	log.Println("npx.cmdを探します...")
	npxpath := searchFile("npx.cmd")
	log.Println("npx.cmdを探しました。")
	log.Println("npx.cmdのパス: ", npxpath)

	log.Println("カレントディレクトリをjaowebに変更します。")
	os.Chdir("jaoweb/")

	log.Println("依存パッケージをダウンロードします。")
	err := exec.Command(npxpath, "yarn", "install").Run() // Ctrl+Cしたときこのプロセスが残る不具合あり
	if err != nil {
		log.Fatal(err)
		return
	}
	log.Println("依存パッケージをダウンロードしました。")

	log.Println("開発サーバを起動します。")
	log.Println("Listening on: と表示されたら、表示されたURLにブラウザからアクセスして下さい。")
	yarndev := exec.Command(npxpath, "yarn", "dev") // Ctrl+Cしたときこのプロセスが残る不具合あり
	yarndev.Stderr = os.Stderr
	yarndev.Stdout = os.Stdout
	yarndev.Stdin = os.Stdin
	yarndev.Start()
	defer yarndev.Process.Kill() // Ctrl+Cの場合動作しない？

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

func downloadNodeJS() string {
	if _, err := os.Stat("node"); os.IsNotExist(err) {
		os.Mkdir("node", 0644)
	}

	nodepath := searchFile("node.exe")
	if nodepath != "" {
		return nodepath
	}

	zippath := "node/node.zip"
	url := "https://nodejs.org/dist/latest-fermium/node-v14.17.0-win-x64.zip"
	downloadFile(zippath, url)

	distpath := "node/"
	unzip(zippath, distpath)

	nodepath = searchFile("node.exe")
	if nodepath != "" {
		return nodepath
	}

	return ""
}

func searchFile(filename string) string {
	var npmpath string
	filepath.Walk(".", func(ret *string) filepath.WalkFunc {
		return func(path string, f os.FileInfo, err error) error {
			if strings.HasSuffix(f.Name(), filename) {
				*ret = path
			}
			return nil
		}
	}(&npmpath))
	if npmpath != "" {
		npmpath, _ = filepath.Abs(npmpath)
	}
	return npmpath
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
