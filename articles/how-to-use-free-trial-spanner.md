---
title: "無料で試す！今から始める Cloud Spanner"
emoji: "🔧"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [cloudspanner, spanner, googlecloud, gcp, database]
publication_name: "google_cloud_jp"
published: true
---

# はじめに

この記事は [Google Cloud Japan Advent Calendar 2022 の「今から始める Google Cloud」編](https://zenn.dev/google_cloud_jp/articles/12bd83cd5b3370)の 9 日目の記事です。

こんにちは、Google Cloud でデータベース系を担当している佐藤です。

本記事では Cloud Spanner に興味がある方に、無料トライアル インスタンスを使ってサクッと触ってもらう手順を紹介します。

本手順は以下のドキュメントをより丁寧に紹介した内容です。

https://cloud.google.com/spanner/docs/free-trial-quickstart?hl=ja

## そもそも Cloud Spanner って何だっけ？
[Cloud Spanner](https://cloud.google.com/spanner?hl=ja)、名前こそ聞くものの、その実態が何なのか知らない方も多いかもしれません。何ならリレーショナル DB だってことを知らない人もいると聞いたので、簡単に紹介したいと思います。

Cloud Spanner の特徴をあげろと言われたら、僕はよくこの 3 つをあげています。
- 運用が簡単（運用することがほぼ無い）
- 可用性が高い（99.999% の可用性を実現）
- 書き込みのスケールアウトができる

なんでこんな特徴が実現できているかというと、Cloud Spanenr は、**ゾーンやリージョンをまたいだ同期レプリケーション** と、 **負荷状況にあわせた自動シャーディング**、この 2 つの運用を **自動化** した分散データベースだからです。

どんな使われ方しているかというと、自前でシャーディングする必要がなくなるので、手動シャーディング運用をしたくない方がよく使われてます。シャードのマージも自動で行ってくれるので、スケールアウトだけでなくスケールインもできちゃいます。可用性についてはほんとうにダウンタイムがない（DB の再起動みたいな運用が一切不要）ので、性能は特に困ってないのだけど、とにかくダウンタイムを限りなく無くしたいという方も使われていますね。

# Cloud Spanner の無料トライアル インスタンス
## 無料トライアル インスタンスの登場
Cloud Spanner に興味がある人はたくさんいると思うのですが、ちょっと試してみたいとき、特に個人で独学で勉強しようと思った時、価格的にお試しするハードルが高かったのも事実です。その後インスタンスの最小単位が小さくなり、[0.1 ノードで月 65 ドル程度から使えるようになった](https://cloud.google.com/blog/ja/products/databases/get-more-out-of-spanner-with-granular-instance-sizing)とはいえ、ちょっと試してみたい個人には避けたい出費です。

そんな方に待望の情報が今年の 9 月 8 日に登場しました。学習用途で使える無料トライアル インスタンスのリリースです。

![2022年9月8日のリリースノート](/images/articles/how-to-use-free-trial-spanner/release-note.png)
*2022年9月8日に無料トライアル インスタンスがリリース*

Cloud Spanner 1 インスタンスを 90 日間無料で試せるというものになります。お試し用なのでインスタンス サイズは小さいですが、それでも 10 GB のストレージがつきますし、十分活用できるものになっています。


## 無料トライアル インスタンスの注意点と制限事項
> The free trial instance is intended to help you learn and explore Spanner.

無料トライアル インスタンスは、主に Cloud Spanner のコンセプトなどを学んで貰うための学習用途が想定されています。また性能測定を想定して作られていません（小さい処理性能しかありません）。あまり強い負荷を書けないように注意してください。Cloud Spanner は強い負荷がかかると、[リクエストをクライアント側にプッシュバック](https://cloud.google.com/spanner/docs/bulk-loading?hl=ja#pushback)します。

主な制限事項は以下の通りです。
1. 無料トライアル インスタンスを作れるのは、1 つのプロジェクト内では 1 度限りです
2. 複数のプロジェクトで同時に作成する場合、請求先アカウントごとに最大 5 つの無料トライアル インスタンス同時に利用できます
3. 無料トライアル インスタンスは 90 日間使えます
4. 1 つの無料トライアル インスタンスの中には最大 5 つのデータベースを作れます
5. SLA の保証はありません。


特に上 2 つをまとめると。**プロジェクトごとに 1 回しか作れないけど（さらにプロジェクトをまたいでも同時に 5 つまで）、古いインスタンスを削除すれば、また新しいプロジェクトでは作れる** ということです。個人で検証する分には十分ですね！



# 実際に無料トライアル インスタンスを試してみる
今回は僕の個人アカウントで試してみました。


## 無料トライアル インスタンスの作成準備
まず無料トライアル インスタンスを試すための Google Cloud のプロジェクトを用意してください。もし今回 Google Cloud を触るのが初めてという方は、[こちらの記事](https://zenn.dev/google_cloud_jp/articles/8cbf93f7a4c7c1#google-cloud-%E7%84%A1%E6%96%99%E3%83%88%E3%83%A9%E3%82%A4%E3%82%A2%E3%83%AB-%E3%81%B8%E3%81%AE%E7%99%BB%E9%8C%B2)で最初のやり方が紹介されています。

今回は新規に用意しています。既存のものを使っても構いません。ただし **無料トライアル インスタンスは 1 プロジェクトごとに 1 回しかつくることができない** ことに注意してください。気になるなら、新規プロジェクトを作ってしまったほうがいいでしょう。

今回僕は free-spanner-202212 というプロジェクト名で作っていますが、皆さんは好きな名前で作ってください。

![](/images/articles/how-to-use-free-trial-spanner/how-to-start01.png)
*新しく作った Google Cloud のプロジェクト*

左のメニューから `Spanner` を選択して、Spanner の管理コンソールに移動しましょう。
**`無料トライアル を開始`** ボタンがあると思います。こちらのボタンから無料トライアル インスタンスを作成できます。1 度無料トライアル インスタンスを作ると、このボタンは消えてしまいます。なお、その下にあるプロビジョニングされたインスタンスとは、通常の有料で使うインスタンスのことです。

![](/images/articles/how-to-use-free-trial-spanner/how-to-start02.png)
*Cloud Spanner の管理コンソール*


**`無料トライアル を開始`** ボタンをクリックして、インスタンスの作成に必要な情報を入力する画面へ移動しましょう。

:::message
`無料トライアルを開始` ボタンをクリックした直後、以下のような表示がでたら、作成したプロジェクトに `請求先アカウント（Billing Account）` がまだ設定されていません。料金が発生した際の支払い設定がされていないということですね。無料で使いたいから紐付けたくないという気持ちはありますが、こちらは[設定](https://cloud.google.com/spanner/docs/free-trial-quickstart?hl=ja#before_you_begin)しておいてください。Google Cloud を[初めてお使いの方は、90 日間 300 ドルまで使えるクレジットが付与](https://cloud.google.com/free/docs/free-cloud-features#free-trial)されますので、そちらも併せて使えます！
![](/images/articles/how-to-use-free-trial-spanner/enable-billing.png)
:::

## インスタンス作成に必要な情報を入力
インスタンス作成には 3 つだけ入力が必要です。

- インスタンス名（ただの表示名なので日本語でも構いません）
- インスタンス ID（インスタンスを一意に特定する ID です）
- 構成（どこのリージョンを使うか）

今回は名前と **ID を `free-instance`、構成を `asia-southeast2（ジャカルタ）`** にしましょう。

通常の有料インスタンスの場合は、上記に加えて `コンピューティング容量` の入力があります。コンピューティング容量とはインスタンスが持っている処理性能のことで、非常にざっくり言ってしまえばほぼノード数のことです。無料トライアル インスタンスは処理性能は固定されているので入力はありません。現時点では無料トライアル インスタンスの処理性能は、Cloud Spanner 1 ノードのおよそ 1/20 程度です。

構成、すなわちリージョンですが、残念ながら無料トライアル インスタンスでは東京リージョンは選択できず、`デリー`、`ジャカルタ`、`フランクフルト`、`コロンバス` の 4 つから選択します。どれを使っても構いませんが、日本から一番ネットワーク的に近いのは `asia-southeast2（ジャカルタ）` になりますので、ジャカルタにしておきましょう。

`無料トライアル インスタンスを作成` ボタンをクリックすると、数秒で作成完了します。

![](/images/articles/how-to-use-free-trial-spanner/how-to-start03.png)
*作成に必要な情報は 3つ*

作成が完了すると以下のような画面に移行します。このあとはデータベースを作って、好きにテーブルを作ったり SQL をなげたりできます。ここでは `チュートリアルを起動する` をクリックしてみます。

![](/images/articles/how-to-use-free-trial-spanner/how-to-start04.png)
*インスタンス作成完了*

# データベースを使ってみる（チュートリアル）

無料トライアル インスタンスで試すためのチュートリアルが用意されています。`チュートリアルを起動する` ボタンをクリックすると、Cloud Console の右側にチュートリアル画面が起動します。そのままそのチュートリアルの手順にそってもらってもいいのですが、本記事ではそのチュートリアルの内容を少し補足しながら紹介したいと思います。

![](/images/articles/how-to-use-free-trial-spanner/tutorial01.png)
*右側にチュートリアル画面が起動している*


## Cloud Shell の起動

まず Cloud Spanner へ接続するアプリを起動する環境を準備します。今回は環境を揃えるために Cloud Shell を利用しましょう。もちろん Compute Engine や、皆さんの手もとの PC 上からも簡単に接続できますが、まずは Cloud Shell で試してみてください。

画面右上の Cloud Shell のアイコンをクリックすると、下にターミナルが開かれます。これが Cloud Shell です。Google Cloud でよく使われる各種コマンドがインストール済みの閉じた環境なので、簡単かつ安全に利用することができます。

![](/images/articles/how-to-use-free-trial-spanner/tutorial02.png)
*Cloud Shell の起動*


## 認証情報の設定

Cloud Spanner への接続認証はユーザー名/パスワードといった方式ではなく、アプリケーションが利用する IAM によって行われます。

このチュートリアルで使うサンプル アプリケーションは、[ユーザー認証情報](https://cloud.google.com/docs/authentication/provide-credentials-adc?hl=ja#local-dev)を使って Cloud Spanner に接続しています。以下のコマンドを実行してユーザー（あなた）の認証情報をアプリケーションが実行される環境上に読み込みます。

```
gcloud auth application-default login
```

![](/images/articles/how-to-use-free-trial-spanner/tutorial03.png)
*gcloud auth application-default login 実行後の画面*

続けるか Y / n が聞かれていますね。そのまま Y を選択し処理を進めます。

:::message
`Y` を選んで処理を進めましたが、そこの英文をよく読んでみると、「あなたの環境（Cloud Shell）では必要な認証情報は自動で読み込まれるから、今実行したコマンドは不要ですよ、それでも実行しますか？」といった内容が書かれています。実は Cloud Shell 環境では本来 `gcloud auth application-default login` を実行しなくても、自動で認証情報が読み込まれているのですが、今回利用するサンプル アプリケーションは、2022-12-09 現在、このコマンドを実行しないとエラーが出てしまうため、このチュートリアルではコマンドを実行しています。なお Cloud Shell 以外、例えばあなたの PC 上からサンプル アプリケーションを実行する場合は、このコマンドを実行して認証情報を読み込む必要があります。
:::

すると以下のように URL が出てきます。クリックしてみましょう。ブラウザで隣のタブに **`アカウントの選択「Google Auth Library」に移動`** の画面が表示されています。今回利用してる Google アカウントをクリックしてください。

```
Do you want to continue (Y/n)?  Y

Go to the following link in your browser:

    https://accounts.google.com/o/oauth2/auth?response_type=code&client_id=.....&code_challenge_method=S256

Enter authorization code:
```


続いて、**`Google Auth Library が Google アカウントへのアクセスをリクエストしています`** という画面が表示されますので、内容を確認して **`許可`** をクリックします。（このように認可を求められる画面は他でも見たことあると思いますが、どのような権限を求められているか確認する癖をつけましょう！）

すると、下図の一番右の画面がでてきて、認証コード（authorization code）が表示されます。`Copy` をクリックしてコピーします。Cloud Shell 上の `Enter authorization code: ` にペーストして入力します。

![](/images/articles/how-to-use-free-trial-spanner/tutorial04.png)
*今回のプロジェクトで利用しているアカウントを選択*


入力すると、以下のように表示されて完了です。

![](/images/articles/how-to-use-free-trial-spanner/tutorial05.png)
*gcloud auth application-default login の諸々が完了した画面*

## サンプル アプリケーションの初期設定

続いてサンプル アプリケーションの初期設定を行います。このサンプル アプリケーションはあらかじめ  Cloud Spanner 向けに用意されたものであり、`gcloud spanner samples` コマンドから実行できます。

Cloud Shell 上で以下のコマンドを実行しましょう。サンプル アプリケーションのダウンロードと、サンプル データベースの作成が行われます。

```bash
gcloud spanner samples init finance --instance-id free-instance
```

Cloud Shell に以下のように表示され、アプリのダウンロードと、サンプル データベースの作成が完了します。この時点でデータベースとテーブルまで作られた状態です。

![](/images/articles/how-to-use-free-trial-spanner/tutorial06.png)
*サンプル データベース finance-db が作成完了*

## サンプル データベースのテーブル構成

ちなみに今回自動で作成された `finance-db` はこのようなテーブル構成になっています。銀行の DB を簡易的に表現したものになります。
![](https://github.com/GoogleCloudPlatform/cloud-spanner-samples/blob/main/finance/images/FinAppERD.png?raw=true)
*サンプル データベース finance-db の ER 図*


## サンプル アプリケーションの実行（バックエンド）

先程の実行結果に、`Next, start the backend gRPC service with:` と書かれています。バックエンドの gRPC サービスをスタートしろとあります。このサンプル アプリケーションは、バックエンドの `FinApp Server` とそこに接続してワークロードを実行する `Workload Generator` から構成されているため、次はこの `FinApp Server` を起動しろということのようです。

![](https://github.com/GoogleCloudPlatform/cloud-spanner-samples/blob/main/finance/images/FinAppConnect.png?raw=true)
*サンプル アプリケーションの概要図*

最終的にはバックエンドの `FinApp Server` と、そこに接続してワークロードを実行する `Workload Generator` 両方を実行するため、この時点で Cloud Shell を 2 つ開きましょう。Cloud Shell ターミナル右上に `+` ボタンがあります。ここをクリックすると Cloud Shell のタブを増やして、複数のコマンドを同時に実行できます。

![](/images/articles/how-to-use-free-trial-spanner/tutorial07.png)
*Cloud Shell を 2 つ開く*

2 つ開いたタブの片方で以下のコマンドを実行し、バックエンドの `FinApp Server` を起動します。無事起動すると 8080 ポートでサーバー起動した旨がログにでます。

```bash
gcloud spanner samples backend finance --instance-id free-instance
```

![](/images/articles/how-to-use-free-trial-spanner/tutorial08.png)
*バックエンドを実行した様子*

:::message alert
上記コマンド実行したけれど `Exception in thread "main" java.lang.NoSuchMethodError: 'io.grpc.alts.GoogleDefaultChannelCredentials$Builder io.grpc.alts.GoogleDefaultChannelCredentials.newBuilder()'` というエラーが出てうまく実行できない場合は、`認証情報の設定` セクションに戻り `gcloud auth application-default login` コマンドを実行し直して見てください。分かりづらいですが、2022-12-09 時点ではこのコマンドを実行しないと、うまくサンプル アプリケーションが動作してくれません。

:::

## サンプル アプリケーションの実行（ワークロード）

次にもう片方のタブに戻り、以下のコマンドを実行してワークロードを生成しましょう。これはサンプル データベースに実際にデータを書き込むアプリケーションです。動き始めると、INSERT が実行されているログが表示されます。

```bash
gcloud spanner samples workload finance
```

![](/images/articles/how-to-use-free-trial-spanner/tutorial09.png)
*ワークロードを実行した様子*


## Cloud Spanner 側でワークロードの様子を見てみる

Cloud Spanner の管理コンソールを見てみましょう。データベースのところの右端にある `更新` をクリックすると、先程作成されたデータベースが表示され、CPU 使用率を見ると実際にワークロードが発生していることが分かると思います。あとは `finace-db` の画面に入り、好きにいろいろ見てまわってみてください。
![](/images/articles/how-to-use-free-trial-spanner/tutorial10.png)
*サンプル アプリケーション実行中の Cloud Spanner のコンソール*


# おわりに

これで実際にデータが入ったサンプル データベースが作れました！次回はこのサンプル アプリケーションを実行したままいろんな画面を見て回ったり、データベースに直接様々なクエリを投げて遊んでみましょう。

# 付録：関連ドキュメント
https://cloud.google.com/spanner/docs/free-trial-instance?hl=ja

https://cloud.google.com/spanner/docs/free-trial-quickstart?hl=ja

https://github.com/GoogleCloudPlatform/cloud-spanner-samples