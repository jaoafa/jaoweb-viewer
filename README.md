# jaoweb-viewer (α)

[jaoafa/jaoweb-docs](https://github.com/jaoafa/jaoweb-docs) の更新作業時用ビュアー

## 機能

- `localhost:8080` でプレビュー表示
- 起動時に最新の [jaoweb](https://github.com/jaoafa/jaoweb) をクローンし、`jaoweb` ディレクトリに配置
- `.gitignore` に `jaoweb-viewer*` と `jaoweb/` がない場合に自動追加する
- 内容の更新時にリロード

## 動作フロー

1. `git` の存在確認
2. `.gitignore` に追加
3. `latest-fermium` の nodejs ポータブルをダウンロード
4. `jaoweb` フォルダが存在しない場合は [jaoafa/jaoweb](https://github.com/jaoafa/jaoweb) のクローン、存在する場合は `git pull`
5. `yarn install` による依存パッケージのダウンロード
6. `yarn dev` による開発サーバの起動

## 既知の不具合

- `yarn dev` で起動する NodeJS のアプリケーションが停止後も動作し続ける
- ファイルの削除・リネーム時に動作しない
