---
title: "Cloud Spanner の SQL ログを gRPC レイヤーで取得する方法"
emoji: "🔧"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [cloudspanner, spanner, googlecloud, gcp, database]
publication_name: "google_cloud_jp"
published: true
---

# はじめに

この記事は [Google Cloud Japan Advent Calendar 2022 の「通常版」](https://zenn.dev/google_cloud_jp/articles/12bd83cd5b3370#%E9%80%9A%E5%B8%B8%E7%89%88)の 21 日目の記事です。

こんにちは、Google Cloud でデータベース系のプロダクトを担当している佐藤です。


## TL;DR - 最初にまとめ

本記事では以下の内容が書かれています。今回は Cloud Spanner 用のアプリケーションの話で例示していますが、gRPC を使う他のアプリにも応用ができる内容になっています。

**本記事の内容**
- アプリケーションが Cloud Spanner へ投げる SQL 及び mutation とパラメータは、gRPC レイヤーでまとめて取得することができる
- gRPC には Interceptor という、各 RPC のリクエストごとに任意の処理を割り込ませる仕組みがある
- Interceptor で Cloud Spanner 用のアプリが発行している SQL とパラメータを取得して、そのままログに吐き出す（Cloud Logging に送るなど）ことができる
- Cloud Logging と Log Analytics (Preview) を活用し、アプリが投げてる SQL 及び　mutation とパラメータを簡単に可視化できる

**本記事で登場する製品やキーワード**
- Go 言語
- gRPC Interceptor
- Cloud Spanner
- Cloud Logging
- Log Analytics

## この記事のきっかけ
Cloud Spanner に接続しているアプリの改修で、「Cloud Spanner に対して発行してる SQL 及び mutation とそのパラメータ（WHERE 句の条件とかの実際の値）をログできるようにしてほしい」と言われたら、どのように実装しますか？ぱっと思いつく方法としては、実際にクエリ投げてるところでログを取れば行けそうですね。

例えばアプリから[パラメータ付きの SELECT](https://cloud.google.com/spanner/docs/samples/spanner-query-with-parameter?hl=ja)を投げる場合は、Go 言語では以下のように記述します。少なくともコード上ではここに SQL テキストと、実際のパラメータの中身があるので、これを任意の方法でログれば良さそうに感じます。

```go: アプリから Cloud Spanner へパラメータ付きの SELECT を
stmt := spanner.Statement{
    SQL: `SELECT SingerId, FirstName, LastName FROM Singers 
            WHERE LastName = @lastName`, // SQL テキスト
    Params: map[string]interface{}{
            "lastName": "Garcia", // パラメータに格納される値
    },
}
iter := client.Single().Query(ctx, stmt)
```

でもアプリがすでにあって、今から追加で**すべての箇所の** SQL テキストとパラメータをログに吐くような変更を加えるのはとても手間です。抜け漏れがでる可能性もあります。何かシンプルにできないでしょうか？

本記事ではそんなときの解決策の 1 つを紹介したいと思います。




# Cloud Spanner と gRPC
## Cloud Spanner は通信に gRPC を利用している
自分は（現職ではなく）昔も似たよう場面に出くわしました。その時は、アプリが発行してる SQL のうち、特定の条件に合致するものを収集するような処理が必要とされた場面でした。しかしアプリは C 言語で書かれており、アプリ自体に改修を入れることは NG でした。そのためアプリが使っていた DB 接続用ライブラリで SQL をフックして SQL テキストとパラメータを取得したことがありました。

Cloud Spanner はどうでしょうか？Cloud Spanner 用のアプリは、各種クライアント ライブラリを利用しています。さらに言えばどのクライアント ライブラリも **gRPC を共通して使っています**。gRPC レイヤーでなんとかできないでしょうか？そう、丁度いい仕組みがあるんです。それが今回利用する gRPC Interceptor です。



## gRPC Interceptor とは
gRPC Interceptor とは、gRPC の通信に対して `Intercept (割り込み)` を行う仕組みです。ざっくりいうと gRPC のメソッドにおいて、その前後に任意の処理を挟み込む事ができます。たとえば gRPC には、リクエストとレスポンスが 1:1 になる Unary RPC と、1:N (もしくは N:1)になる Streaming RPC がありますが、それぞれに Interceptor の仕組みが用意されています。

gRPC Interceptor 自体の詳細な解説は、今回の本題ではないので端折ります。`gRPC Interceptor` で検索すると、世間の素晴らしいブログ記事がたくさんでてきますので、興味がある方はそちらをご覧ください。


## Cloud Spanner が使う gRPC のメソッド例

さて Cloud Spanner に SQL を投げるときはどんな gRPC を使っているのでしょうか？
[google.spanner.v1.Spanner](https://cloud.google.com/spanner/docs/reference/rpc#google.spanner.v1.spanner) にアプリが Cloud Spanner で読み書きを行う際のメソッドが載っています。

代表的なものを以下に抜き出してみました。

| メソッド               | 種類           | 説明
| --------------------- | ------------- | -------------------
| BeginTransaction      | Unary RPC     | トランザクションの開始
| Commit                | Unary RPC     | トランザクションのコミット
| Rollback              | Unary RPC     | トランザクションのロールバック
| ExecuteBatchDml       | Unary RPC     | 複数 DML のバッチ実行
| ExecuteSql            | Unary RPC     | SQL の実行
| ExecuteStreamingSql   | Streaming RPC | SQL の実行（ストリーム受信）
| Read                  | Unary RPC     | key/value 形式での行の読み取り
| StreamingRead         | Streaming RPC | key/value 形式での行の読み取り（ストリーム受信）


Unary RPC 用の Interceptor と Streaming RPC 用の Interceptor で、上記について中身を取り出せば、SQL テキストやパラメータが取れそうです。

:::message
おや、mutation API による更新用のメソッドがないじゃないかと思わた方ご安心を。mutation による更新は、Commit リクエストに更新内容を乗せて行っています。[こちらの記事](https://medium.com/google-cloud-jp/%E8%A9%B3%E8%A7%A3-google-cloud-go-spanner-%E3%83%88%E3%83%A9%E3%83%B3%E3%82%B6%E3%82%AF%E3%82%B7%E3%83%A7%E3%83%B3%E7%B7%A8-6b63099bd7fe)に詳しい解説がございます。
:::



# 試してみよう

実際に試してみましょう。今回は Cloud Spanner の Go 言語用のサンプルアプリを利用して、サンプルアプリが投げる各種 SQL を、Cloud Logging に記録してみます。

:::message alert
本記事で紹介するコード例はあくまでサンプルコードです。本記事の中で試すこと以外での利用は一切想定しておりません。あくまで参考までにとどめ、そのまま再利用しないようお気をつけください。

また、本記事で紹介する手法は、SQL や Mutation に含まれる実際のパラメータをログに取ることが可能になります。PII（個人を特定できる情報）などが含まれうる情報の記録については、必ず所属する組織のポリシーやアプリケーションの要件を確認することを強く推奨します。

Cloud DLP を利用した機密情報補語方法の例が[こちらのブログ](https://medium.com/google-cloud/protect-sensitive-info-in-logs-using-google-cloud-4548211d4654)にあります。
:::

## 今回利用する仕組みの概要図

Cloud Spanner に接続するアプリは Cloud Spanner 用のクライアント ライブラリを使っています。アプリで SELECT を発行すると、クライアント ライブラリ経由で gRPC の通信として Cloud Spanner の API エンドポイントへ飛んでいきます。


![](/images/articles/how-to-intercept-sqls-and-params/architecture.png)
*今回の利用する仕組みの概要図*

## 元となるサンプルアプリ snippet.go の用意

まずは Cloud Spanner インスタンスを用意します。今回は無料トライアル インスタンスで試してみます。また、今回は手順を統一するため、Cloud Shell 上でアプリを動かしたいと思います。

Cloud Shell を起動し、以下のコマンドを実行してください。まずは元にするサンプルアプリをダウンロードします。

```shell: Go 言語用サンプルアプリのダウンロード
git clone https://github.com/GoogleCloudPlatform/golang-samples
cd golang-samples/spanner/spanner_snippets
```

今回元にするアプリは、[こちらのドキュメントのチュートリアル](https://cloud.google.com/spanner/docs/getting-started/go?hl=ja)にも使われてる、`spnippet.go` です。`go run snippet.go query` のように、スニペットで用意されている各種コードを、お手軽に試せるサンプルアプリになります。

https://cloud.google.com/spanner/docs/getting-started/go?hl=ja

先程 `git clone` したあと、spanner_snippets というディレクトリに移動しています。以降このディレクトリで操作を行います。

```shell:利用するサンプルアプリのディレクトリ構造
golang-samples
  ├─ spanner
      ├─ spanner_snippets # このディレクトリに cd して各種操作を行う
          └─ snippet.go # 元にするサンプルアプリ
```

まずは今回利用する Google Cloud 環境の `プロジェクト ID / インスタンス ID / DB 名` を Cloud Shell 上の環境変数に入れておきます。今回インスタンス名は `free-instance` にしていますが、異なる ID を使う場合は各自自前のインスタンス名に置き換えてください。echo の結果を確認し、正しい `プロジェクト ID / インスタンス ID / DB 名` が入っていることを確認します。

```shell: 環境変数の設定
export PROJECT_ID=$(gcloud config list project --format "value(core.project)")
export INSTANCE_ID="free-instance"
export DB_NAME="example-db"

echo ${PROJECT_ID}
echo ${INSTANCE_ID}
echo ${DB_NAME}
```

## サンプルアプリ経由で DB 作成

次に snippet.go 経由で、DB を作成します。以下の **`DB の作成コマンド`** を実行してください。途中 `Cloud Shell の承認` というポップアップがでてきますので、承認ボタンをクリックしてください。DB の作成には十数秒かかります。成功すると `Created database` と表示されます。

```shell: DB の作成コマンド
go run snippet.go createdatabase projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
```

この DB にはまだデータが入っていませんので、同じく snippet.go の機能を使って **`サンプルデータの格納コマンド`** を実行し、サンプルデータを数件入れてみます。なおこの一連のコマンドは、元々はコマンド名の通り DML 書き込みや UPDATE など、各種操作の参考用のスニペットです。

```shell: サンプルデータの格納コマンド
go run snippet.go dmlwrite projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
go run snippet.go write projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
go run snippet.go addnewcolumn projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
go run snippet.go update projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
```

`addnewcolumn` コマンドは `ALTER TABLE ADD COLMUN` で列を足しているため、このコマンド完了には十数秒かかります。

4 つのコマンドを実行しデータ投入が終わったら、以下の **`サンプルデータの確認コマンド`** コマンドを使って実際に格納されているサンプルデータにクエリを投げてみましょう。

```shell: サンプルデータの確認コマンド
go run snippet.go query projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
go run snippet.go querywithparameter projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
```

それぞれ以下のような出力が得られるはずです。これで DB とデータの準備は完了です。

```shell:go run snippet.go query の結果
1 1 Total Junk
1 2 Go, Go, Go
2 1 Green
2 2 Forever Hold Your Peace
2 3 Terrified
```

```shell:go run snippet.go querywithparameter の結果
12 Melissa Garcia
```
## サンプルアプリに gRPC Interceptor を仕込む

さてここからが本題です。gRPC Interceptor の処理はどのようなコードを書けばいいのでしょうか？**まずは手っ取り早く試してもらうために、[こちらに](https://raw.githubusercontent.com/takabow/zenn-articles/main/src/articles/how-to-intercept-sqls-and-params/logging.go) `snippet.go` に簡単に gRPC Interceptor を組み込める `logging.go` という名前のサンプルコードを用意しておきました！**

今いるディレクトリで以下の `wget` を実行して、[logging.go](ttps://raw.githubusercontent.com/takabow/zenn-articles/main/src/articles/how-to-intercept-sqls-and-params/logging.go) というファイルをダウンロードします。今回の記事用に用意したシンプルなサンプルコードとなっています。

:::message alert
再掲：本記事で紹介するコード例はあくまで　snippet.go に簡単に組み込めることを目的としたサンプルコードです。本記事の中で試すこと以外での利用は一切想定しておりません。あくまで参考までにとどめ、そのまま本番環境等で再利用しないようお気をつけください。
:::

```shell: サンプルデータの確認コマンド
wget https://raw.githubusercontent.com/takabow/zenn-articles/main/src/articles/how-to-intercept-sqls-and-params/logging.go
```

結果としてディレクトリ構成は以下のようになります。

```shell:サンプルコードのディレクトリ構造
golang-samples
  ├─ spanner
      ├─ spanner_snippets　# このディレクトリに cd して各種操作を行う
          ├─ snippet.go # 元にするサンプルアプリ
          └─ logging.go # 今ダウンロードした Interceptor 用のコード
```

Intercept（割り込み）を行うコード自体は `logging.go` に書いてあります。それを `snippet.go` 内の Cloud Spanner クライアントに渡すことで、gRPC Interceptor を実現します。まずは動かして試してみましょう。コードの説明は後述します。

組み込むために `snippet.go` を **3 行をほど修正**しなくてはいけませんので、まずはエディタを開きます。以下のコマンドを実行すると、カレントディレクトリのファイルを編集できる [Cloud Shell エディタ](https://cloud.google.com/shell/docs/editor-overview)が起動します。`snippet.go` が開かれた状態になると思います。

```shell: エディタの起動
cloudshell workspace .
```

Cloud Shell エディタを使って snippet.go ファイルをこれから説明する通りに編集してみましょう。うまく snippet.go が開かれなかった場合は、左部のメニューから自分で開いてみてください。もちろん vim や emacs など好みのエディタで編集しても構いません。

### snippet.go の修正（main 関数）

main 関数のなかで最初に Cloud Logging の初期化と終了処理が必要になります。これはダウンロードした `logging.go` の中にある `gRPCLoggerStart()` と `gRPCLoggerStop()`関数を呼び出すと行ってくれるようにしてあります。この 2 行を追加しましょう。`defer gRPCLoggerStop()` とすることで、main 関数終了時に、まだ Cloud Logging に送信されてないログが flush されるようにしてあります。

```diff go:snippet.go - main()
	cmd, db := flag.Arg(0), flag.Arg(1)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
+	gRPCLoggerStart(ctx, db)
+	defer gRPCLoggerStop()
	adminClient, dataClient := createClients(ctx, db)
	defer adminClient.Close()
	defer dataClient.Close()
```

### snippet.go の修正（createClients 関数）

次に、createClients 関数を修正し、gRPC Interceptor 用の関数をセットした opts を Cloud Spanner クライアントに渡せるようにします。ダウンロードした `logging.go` の中にある `getInterceptOpts()` 関数が opts を返してくれるます。spanner.NewClient の第 3 引数に `getInterceptOpts(ctx)...` を追加するように修正します。これで完成です。

```diff go:snippet.go - createClients()
func createClients(ctx context.Context, db string) (*database.DatabaseAdminClient, *spanner.Client) {
	adminClient, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

-	dataClient, err := spanner.NewClient(ctx, db)
+	dataClient, err := spanner.NewClient(ctx, db, getInterceptOpts(ctx)...)
	if err != nil {
		log.Fatal(err)
	}
	return adminClient, dataClient
}
```

## Interceptor を仕込んだアプリの実行

エディタの上部メニューにある「ターミナルを開く」をクリックし、ターミナルに戻り以下のコマンドを実行します。今回ダウンロードした logging.go が必要としてる依存パッケージを解決します。

```shell: logging.go が必要としてる依存を解決する
go mod tidy
```

そして snippet.go で `querywithparameter` を実行してみましょう。新しく足した logging.go を引数に追加し、`go run snippet.go logging.go` とするのをお忘れなく。

```shell: gRPC Interceptor を有効にしての querywithparameter の実行
go run snippet.go logging.go querywithparameter projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
```

以下のような結果が出るはずです。
`[/google.spanner.v1.Spanner/ExecuteStreamingSql]` と出力されていますね。これが実際に SQL を Cloud Spanner に渡しているリクエストです。後ほど解説しますが、今回 gRPC Interceptor の中で、投げられたリクエストのメソッド名をこのように標準出力に出すようにしています。またリクエストに含まれるメッセージについては Cloud Logging に送られています。

```shell: gRPC Interceptor によリ ExecuteStreamingSql クエストが取り出された
[/google.spanner.v1.Spanner/BatchCreateSessions]
[/google.spanner.v1.Spanner/BatchCreateSessions]
[/google.spanner.v1.Spanner/BatchCreateSessions]
[/google.spanner.v1.Spanner/BatchCreateSessions]
[/google.spanner.v1.Spanner/ExecuteStreamingSql]
12 Melissa Garcia
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
[/google.spanner.v1.Spanner/BeginTransaction]
```

:::message

SELECT 文を実行してるだけのはずが、大量の `BeginTransaction` が発行されている様子が見えたりすると思います。BeginTransaction といえばトランザクションの開始時に投げられるリクエストです。SELECT しかしてないはずなのになぜでしょうか？Cloud Spanner のクライアント ライブラリが自動で作るセッション プールでは、書き込み用のセッションというものを用意しており、そちらは最初に BeginTransaction まで投げていつでもトランザクションを開始できる状態にしているのです。詳細は[こちらの記事](https://medium.com/google-cloud-jp/%E8%A9%B3%E8%A7%A3-google-cloud-go-spanner-%E3%82%BB%E3%83%83%E3%82%B7%E3%83%A7%E3%83%B3%E7%AE%A1%E7%90%86%E7%B7%A8-d805750edc75)が詳しいです。
:::

gRPC のメソッドごとにリクエストに割り込みをかけて中身を出力することに成功したようです！


次に snippet.go で `write` を実行してみましょう。これは内部では InsertOrUpdate の mutation を実行してます。

```shell: gRPC Interceptor を有効にしての querywithparameter の実行
go run snippet.go logging.go write projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
```

先ほどと同じようなログが出ますが、よくみると `[/google.spanner.v1.Spanner/Commit]` が出力されていますね。冒頭で説明したとおり、mutation は Commit リクエストにパラメータを渡して実行されます。

```shell: gRPC Interceptor によって取り出された Commit リクエスト
[/google.spanner.v1.Spanner/Commit]
```

この表示はあくまで取れてることを確認するための標準出力のログです。実際の SQL テキストとパラメータは Cloud Logging に送っています。そちらも確認してみましょう！

## Cloud Logging で SQL テキストとパラメータの閲覧

今回はリクエスト内のメッセージを Cloud Logging に送っていたので Cloud Logging を見てみましょう。Message や Method の文字列が入ったログが記録されているのが見えると思います。これが各 gRPC リクエストの情報になります。

![](/images/articles/how-to-intercept-sqls-and-params/cloud-logging01.png)
*Cloud Logging に送られた gRPC の Message*

今回 snippet.go で SELECT を実行しているはずなので、ExecuteStreamingSql のログを探して見ると、しっかりとログが記録されていますね。Cloud Logging は構造化ログといって、ログの構造を維持したまま扱うことができる仕組みがあります。今回は [gRPC のリクエストで送られるメッセージの構造(proto)](https://cloud.google.com/spanner/docs/reference/rest/v1/projects.instances.databases.sessions/executeStreamingSql#request-body)をそのまま記録しています。メッセージの内容は Cloud Logging 上で `jsonPayload` という形で、JSON 形式で扱うことができます。

![](/images/articles/how-to-intercept-sqls-and-params/cloud-logging-sql-params.png)
*SQL とパラメータが記録されている*

また mutation も実行しましたが、そちらは該当　Commit リクエストの Message 内に記録されます。以下の画面のようにこちらも記録されていますね。

![](/images/articles/how-to-intercept-sqls-and-params/cloud-logging-mutation.png)
*mutation とそのパラメータも記録されている*

## Log Analytics での SQL テキストの閲覧

### ログバケットとシンクの作成

さてログが取れてることがわかったので、今回のログだけを保存するログバケットを作って、そこにログを集めてみましょう。以下の内容で新しくログバケットとシンクを作成します。

- ログシンク名：`spanner-sql-log-sink`
- シンク宛先：
  - シンクサービスの選択：Cloud Logging バケット
  - ログバケットの選択：新しいログバケットを作成
    - ログバケット名：`spanner-sql-log-bucket`
    - **Upgrade to use Log Analytics にチェック**
- シンクに含めるログの選択：`log_id("spanner-request-log")`

`log_id("spanner-request-log")` は、snippet.go の中に追加した Cloud Logging の設定で、この名前で設定しています。もし変更する場合は、[コード上の文字列](https://github.com/takabow/zenn-articles/blob/main/src/articles/how-to-intercept-sqls-and-params/logging.go#L26)も変更してください。

![](/images/articles/how-to-intercept-sqls-and-params/cloud-logging03.png)
*ログルーターの設定*


![](/images/articles/how-to-intercept-sqls-and-params/cloud-logging04.png)
*ログルーターの設定*


これでシンクを作成しました。これ以降新しいログはこちらにルーティングされてきます。では再度 snippet.go を実行してログを飛ばしてみましょう。
### Log Analytics でログを整形表示する

`querywithparameter`、`write`、`dmlwritetxn` を実行してみます。再度 Cloud Logging を見てみましょう。無事新しいログが `spanner-sql-log-sink` に格納されているようです。

```
go run snippet.go logging.go querywithparameter projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
go run snippet.go logging.go write projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
go run snippet.go logging.go dmlwritetxn projects/${PROJECT_ID}/instances/${INSTANCE_ID}/databases/${DB_NAME}
```

[Log Analytics](https://cloud.google.com/blog/ja/products/devops-sre/introducing-cloud-loggings-log-analytics-powered-by-big-query?hl=ja) という SQL をつかって Cloud Logging のログを柔軟に検索できる機能（2022 年 12 月現在プレビュー版）があります。これを使うと先程のログを更に見やすく簡単に整形できてしまいます。今回 gRPC Interceptor では、メッセージをそのままの構造で送ってきています。先程 JsonPayload 見たとおり、複雑な JSON 構造をしています。これを Log Analytics で見やすく整形してみましょう。

Cloud Logging の左のメニューから Log Anlytics を選択してください。
![](/images/articles/how-to-intercept-sqls-and-params/log-analytics01.png)
*Log Analytics の画面*

そして以下のクエリを実行してみましょう。SELECT 対象のテーブルの部分は、`<あなたのプロジェクト名>` はあなたのプロジェクト名に置き換えてください。では実行してみましょう。

```sql
WITH spanner_app_logs AS (
  SELECT
    DATETIME(timestamp, 'Asia/Tokyo') AS timestamp,
    SPLIT(JSON_VALUE(json_payload.Method), '/')[OFFSET(2)] AS method,
    JSON_VALUE(json_payload.Message.sql) AS sql_text,
    json_payload.Message.params AS sql_params,
    IF(
      mutations IS NOT NULL,
      SPLIT(TO_JSON_STRING(JSON_QUERY(mutations,'$.Operation')),'"')[OFFSET(1)],
      NULL
    ) AS mutation_type,
    COALESCE(
      JSON_QUERY(mutations,'$.Operation.InsertOrUpdate'),
      JSON_QUERY(mutations,'$.Operation.Update'),
      JSON_QUERY(mutations,'$.Operation.Insert')
    )
    AS mutation_params
  FROM
    `<あなたのプロジェクト名>.global.spanner-sql-log-bucket._AllLogs`
    LEFT JOIN UNNEST(JSON_QUERY_ARRAY(json_payload.Message.mutations)) AS mutations
  WHERE
    timestamp > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 HOUR)
)
SELECT DISTINCT
  timestamp,
  method,
  COALESCE(sql_text, mutation_type) AS sql_or_mutation,
  TO_JSON_STRING(COALESCE(sql_params, mutation_params)) AS params
FROM spanner_app_logs
WHERE method in ('ExecuteStreamingSql','ExecuteSql','Commit')
ORDER BY timestamp, sql_or_mutation
```

以下が結果です！先程実行した 3 つのコマンドについて、 SQL テキストだけでなく、mutation や パラメータまで全て取得できています。Log Analytics を使えば、複雑な構造化ログをこんなに簡単整形できちゃうのです。もちろん今回のケースでは gRPC Interceptor 側である程度分かりきってる部分については前処理してから Cloud Logging に送ってもいいのですが、今回はあえて Log Analytics でやってみました。
![](/images/articles/how-to-intercept-sqls-and-params/log-analytics02.png)
*Log Analytics で SQL とパラメータを表示してみた*




# gRPC Interceptor のコードの説明

最後に logging.go の中では何をやってるのか簡単に紹介して終わりたいと思います。



## Unary RPC の Intercept 処理

冒頭で説明したとおり、Unary RPC (Commit リクエストなど）をフックする処理をここで書きます。`invoker()` が実際に RPC リクエストを投げてるところです。つまりこの前後に処理をかけば、前処理と後処理を割り込ませることができるわけです。

今回はリクエストの送信後に、送ったリクエストの中身の Message を記録するような処理を入れています。

```go:logging.go
// Unary RPC（ExecuteSql など）のための Interceptor
func spannerUnaryClientInterceptor(exporter *sampleExporter) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// ここで実際のリクエストを送信する
		err := invoker(ctx, method, req, reply, cc, opts...)
		// リクエストで送った msg を記録する
		if msg, ok := req.(proto.Message); ok {
			exporter.logMessage(method, msg)
		}
		return err
	}
}
```

関数の細かい引数は、以下のドキュメントに定義があります。
https://pkg.go.dev/google.golang.org/grpc#UnaryClientInterceptor

## Streaming RPC の Intercept 処理

Streaming RPC (ExecuteStreamingSql リクエストなど）をフックする処理をここで書きます。ストリーム処理なので、実際にはリクエストやレスポンスの処理が複数回呼び出される可能性があります。`SendMsg` や `RecvMsg` が実際にそれぞれでの割り込み処理を書く部分です。今回は `ExecuteStreamingSql` リクエスト時の SQL を記録したいので、レスポンスが返ってきたときに記録することにします。なおレスポンスは複数回呼び出されるため、最初の 1 回のみログを取るようにしています。

今回はリクエストの送信後に、送ったリクエストの中身の Message を記録するような処理を入れています。

```go:logging.go
// Streaming RPC（ExequteStreamingSql など） のための Interceptor
func spannerStreamClientInterceptor(exporter *sampleExporter) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		s, err := streamer(ctx, desc, cc, method, opts...)
		// 実際の割り込み処理は SendMsg と RecvMsg でそれぞれ行われる
		return &loggingClientStream{exporter, method, nil, false, s}, err
	}
}

// Streming RPC の中で持ち回る構造体
type loggingClientStream struct {
	exporter *sampleExporter
	method   string
	msg      proto.Message
	logged   bool
	grpc.ClientStream
}

// Streaming RPC のリクエスト送信時の割り込み処理
func (s *loggingClientStream) SendMsg(m interface{}) error {
	if msg, ok := m.(proto.Message); ok {
		s.msg = msg
	}
	return s.ClientStream.SendMsg(m)
}

// Streaming RPC のレスポンス受信時の割り込み処理
func (s *loggingClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	// RecvMsg は複数回呼ばれるので、最初の1つめでのみ記録
	if s.logged {
		return err
	}

	// レスポンス受信が始まったら当初のリクエストを記録する
	s.exporter.logMessage(s.method, s.msg)
	s.logged = true
	return err
}
```

関数の細かい引数は、以下のドキュメントに定義があります。
https://pkg.go.dev/google.golang.org/grpc#StreamClientInterceptor


## ロギング処理

最後にロギング処理です。今回は Cloud Logging に送っていますので、Cloud Logging に送るログ構造を定義しています。gRPC の Message の proto をそのまま渡すだけで、あとは構造化ログとしてうまく処理してくれます。

```go:logging.go
// Cloud Logging にわたす構造化ログ
type spannerClientLog struct {
	Method  string
	Message proto.Message
}

// Cloud Logging にログを書き込む部分
func (exporter *sampleExporter) logMessage(method string, msg proto.Message) {
	exporter.logger.Log(logging.Entry{
		Payload: &spannerClientLog{
			Method:  method,
			Message: msg,
		},
		Severity: logging.Debug,
	})
	fmt.Fprintf(os.Stdout, "[%v]\n", method)
	return
}
```

## 今後の応用

今回はシンプルに全てのリクエストについて Cloud Logging にその Message を送ってみました。
用途が決まっている場合は、この Interceptor 内である程度フィルタリングや Message の整形を行うことも可能です。

- 特定の gRPC メソッド（ExecuteSql や ExecuteStreamingSql など）だけ送る
- Commit リクエスト内の mutation についてはあらかじめパラメータなど取り出してそれらだけ送る
- Cloud Logging ではなく独自のロガーで記録する
- などなど