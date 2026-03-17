# [3.10.0](https://github.com/honeydipper/honeydipper/compare/v3.9.2...v3.10.0) (2026-02-13)


### Bug Fixes

* **deps:** update module github.com/go-git/go-git/v5 to v5.16.5 [security] ([#680](https://github.com/honeydipper/honeydipper/issues/680)) ([0bdc406](https://github.com/honeydipper/honeydipper/commit/0bdc4060c71d0ff2a45da9bbaba12612595b7f38))
* **deps:** update module golang.org/x/crypto to v0.45.0 [security] ([8cab2d7](https://github.com/honeydipper/honeydipper/commit/8cab2d7b6c1255ab32342f3017ca46d01ef37f7d))
* openai in non-streaming mode ([aca7e40](https://github.com/honeydipper/honeydipper/commit/aca7e40461adbcba3cf8bf385bb76d79156a6089))
* tested successfully ([d37a79a](https://github.com/honeydipper/honeydipper/commit/d37a79a63149893fe1b0c8bab20bbd3e8c1d8575))
* tests and changes to support tests ([a0ea244](https://github.com/honeydipper/honeydipper/commit/a0ea244c90b4d3cf35c2ad88e07aa2f22dd26d1c))


### Features

* add openai driver ([6ff362d](https://github.com/honeydipper/honeydipper/commit/6ff362dde6a4db7255fb6ffa480afad5fb1cb9ec))

## [3.9.2](https://github.com/honeydipper/honeydipper/compare/v3.9.1...v3.9.2) (2025-11-05)


### Bug Fixes

* **deps:** update golang.org/x/exp digest to a4bb9ff ([#653](https://github.com/honeydipper/honeydipper/issues/653)) ([29781c5](https://github.com/honeydipper/honeydipper/commit/29781c5295a479c51b3f7f24f87dfd9b7b58789a))
* **deps:** update module dario.cat/mergo to v1.0.2 ([#656](https://github.com/honeydipper/honeydipper/issues/656)) ([f99ae0a](https://github.com/honeydipper/honeydipper/commit/f99ae0ae18239c5ae88b777dd95e2c8a9f1ba457))
* make tzdata available ([#672](https://github.com/honeydipper/honeydipper/issues/672)) ([9f6b556](https://github.com/honeydipper/honeydipper/commit/9f6b55622a6c394803480e10641415d1ee4f6094))
* remove use of time.After ([#673](https://github.com/honeydipper/honeydipper/issues/673)) ([5b28b5b](https://github.com/honeydipper/honeydipper/commit/5b28b5bfa0ede394cadb6b1e118caf91c7f41ce7))
* replace time.After with context Done ([#661](https://github.com/honeydipper/honeydipper/issues/661)) ([ecb3c7a](https://github.com/honeydipper/honeydipper/commit/ecb3c7a6353333868f427c84450461f27ec0731e))

## [3.9.1](https://github.com/honeydipper/honeydipper/compare/v3.9.0...v3.9.1) (2025-05-27)


### Bug Fixes

* use JobSuccessCriteraMet as JobComplete ([#651](https://github.com/honeydipper/honeydipper/issues/651)) ([b11a5a1](https://github.com/honeydipper/honeydipper/commit/b11a5a1f14a1bc254b580dfb0a0faf5064798fdf))

# [3.9.0](https://github.com/honeydipper/honeydipper/compare/v3.8.0...v3.9.0) (2025-05-14)


### Bug Fixes

* **deps:** update golang.org/x/exp digest to 7e4ce0a ([#635](https://github.com/honeydipper/honeydipper/issues/635)) ([f817c72](https://github.com/honeydipper/honeydipper/commit/f817c72d0a1a338d64c956c9c35c2c663467924a))
* **deps:** update google.golang.org/genproto digest to 10db94c ([#630](https://github.com/honeydipper/honeydipper/issues/630)) ([72ab87c](https://github.com/honeydipper/honeydipper/commit/72ab87c52039c2e32f4476f27a55479e65c1a66c))
* **deps:** update module cloud.google.com/go/secretmanager to v1.14.7 ([#643](https://github.com/honeydipper/honeydipper/issues/643)) ([0d70b29](https://github.com/honeydipper/honeydipper/commit/0d70b29a04f44160f1366f7b3ba6d901c13ee252))
* **deps:** update module dario.cat/mergo to v1.0.1 ([#646](https://github.com/honeydipper/honeydipper/issues/646)) ([7aff60c](https://github.com/honeydipper/honeydipper/commit/7aff60c52e3f958d7693d325e21ada4be4f9a047))
* **deps:** update module github.com/golang-jwt/jwt/v5 to v5.2.2 [security] ([#631](https://github.com/honeydipper/honeydipper/issues/631)) ([9223e14](https://github.com/honeydipper/honeydipper/commit/9223e143ef7930b7b792453ebe1d6db6ea95f9aa))
* minor improvement in helper libs ([fffddea](https://github.com/honeydipper/honeydipper/commit/fffddea801d86f9f1dee0b9b4543fd6bf6c30189))


### Features

* allow rpc call with timeout ([c2cab4b](https://github.com/honeydipper/honeydipper/commit/c2cab4b8ba868431f535754a158e4cde60ced83d))
* allow start dynamic workflow session from operator ([aa507c8](https://github.com/honeydipper/honeydipper/commit/aa507c8e76ea7cddf64f17b4cadac909d2b9f6d1))
* driver to interact with gemini ([92cb41e](https://github.com/honeydipper/honeydipper/commit/92cb41e9adb9097e96fd4f5738c1469de896365c))
* driver to interact with ollama ([e253c72](https://github.com/honeydipper/honeydipper/commit/e253c7278ee21daf7d94e765bd4522c54196918e))
* RAG and embeddings driver using GCP vector search and Qdrant ([#650](https://github.com/honeydipper/honeydipper/issues/650)) ([0e45190](https://github.com/honeydipper/honeydipper/commit/0e45190340003e1950e87d3b13676bbbe65af0b6))
* redis-cache handling of queues ([f15ed50](https://github.com/honeydipper/honeydipper/commit/f15ed50257eb39608fa2b02a39bface63ce45ca2))
* sharable common AI chat logic ([0373778](https://github.com/honeydipper/honeydipper/commit/0373778fb7690517b1a86dcb5eccab9045b93857))
* support hide thinking messages for reasoning models ([#647](https://github.com/honeydipper/honeydipper/issues/647)) ([0a1f736](https://github.com/honeydipper/honeydipper/commit/0a1f736d5f8facc9af6ead2794d471d0d8cd080d))
* webhook customized response and replay attack ([6661317](https://github.com/honeydipper/honeydipper/commit/6661317f9417454202cec881fba325e2f09f4a1d))

# [3.8.0](https://github.com/honeydipper/honeydipper/compare/v3.7.0...v3.8.0) (2025-04-01)


### Bug Fixes

* secret lookup should return raw message with bytes ([#632](https://github.com/honeydipper/honeydipper/issues/632)) ([dce4f78](https://github.com/honeydipper/honeydipper/commit/dce4f78a025b2cd771a76abd2eed0529b0ac0289))


### Features

* support GKE DNS endpoint for control plane access ([#633](https://github.com/honeydipper/honeydipper/issues/633)) ([9fb489f](https://github.com/honeydipper/honeydipper/commit/9fb489fec99d580fa205035c11ac2e181b083c95))

# [3.7.0](https://github.com/honeydipper/honeydipper/compare/v3.6.0...v3.7.0) (2025-01-09)


### Bug Fixes

* check k8s job status with conditions array ([#623](https://github.com/honeydipper/honeydipper/issues/623)) ([d4a099c](https://github.com/honeydipper/honeydipper/commit/d4a099c907140de4566de071ca68c0a81574232c))
* **deps:** update golang.org/x/exp digest to 7588d65 ([#617](https://github.com/honeydipper/honeydipper/issues/617)) ([b657453](https://github.com/honeydipper/honeydipper/commit/b6574536be63222a2fd504d76cd9eaed3091800c))
* **deps:** update google.golang.org/genproto digest to 5f5ef82 ([#618](https://github.com/honeydipper/honeydipper/issues/618)) ([218fe31](https://github.com/honeydipper/honeydipper/commit/218fe31ed3439ce35ca931e0e3ad5c4923f13263))
* **deps:** update module github.com/go-git/go-git/v5 to v5.13.0 [security] ([#619](https://github.com/honeydipper/honeydipper/issues/619)) ([1765db9](https://github.com/honeydipper/honeydipper/commit/1765db914a23673d1df7527d510d9907307c22e6))
* **deps:** update module github.com/go-git/go-git/v5 to v5.13.1 ([#621](https://github.com/honeydipper/honeydipper/issues/621)) ([0cbac6a](https://github.com/honeydipper/honeydipper/commit/0cbac6ac232f9f2a067f48eb00347a6468772d0f))
* k8s job only fail if succeeded is 0 ([#622](https://github.com/honeydipper/honeydipper/issues/622)) ([632b72c](https://github.com/honeydipper/honeydipper/commit/632b72ce5e4b180fbe6b314be0c5a9a180b11f2a))


### Features

* accessing vault secrets ([#620](https://github.com/honeydipper/honeydipper/issues/620)) ([b56996f](https://github.com/honeydipper/honeydipper/commit/b56996f0f6bc270cf9d9aba469b1f22b9d47231b))

# [3.6.0](https://github.com/honeydipper/honeydipper/compare/v3.5.0...v3.6.0) (2024-11-25)


### Bug Fixes

* **deps:** update google.golang.org/genproto digest to e639e21 ([#606](https://github.com/honeydipper/honeydipper/issues/606)) ([42cc276](https://github.com/honeydipper/honeydipper/commit/42cc276e1c2255f909f7227f84715611a189eeff))
* **deps:** update module cloud.google.com/go/secretmanager to v1.14.2 ([#608](https://github.com/honeydipper/honeydipper/issues/608)) ([b1f58ac](https://github.com/honeydipper/honeydipper/commit/b1f58ac3507c610a286815403a8fde7a4876a5b8))


### Features

* thottle parallel iteration with pooling ([#611](https://github.com/honeydipper/honeydipper/issues/611)) ([e2c1815](https://github.com/honeydipper/honeydipper/commit/e2c1815b6229045a9bba2dc243c00d5f27a957c2))

# [3.5.0](https://github.com/honeydipper/honeydipper/compare/v3.4.1...v3.5.0) (2024-08-26)


### Bug Fixes

* **deps:** update golang.org/x/exp digest to 2c58cdc ([#579](https://github.com/honeydipper/honeydipper/issues/579)) ([f9113f2](https://github.com/honeydipper/honeydipper/commit/f9113f24fe5d7e5b5dd09d4d5280366555fe38bb))
* **deps:** update golang.org/x/exp digest to 778ce7b ([#596](https://github.com/honeydipper/honeydipper/issues/596)) ([beb6220](https://github.com/honeydipper/honeydipper/commit/beb62203a4bd2383a0c6fa5089301fdf945fce49))
* **deps:** update google.golang.org/genproto digest to a8a6208 ([#597](https://github.com/honeydipper/honeydipper/issues/597)) ([cb83b67](https://github.com/honeydipper/honeydipper/commit/cb83b675ea7c3dff5fbe93b8fe42a5965f75ed95))
* **deps:** update google.golang.org/genproto digest to b0ce06b ([#580](https://github.com/honeydipper/honeydipper/issues/580)) ([6b84223](https://github.com/honeydipper/honeydipper/commit/6b842239e42c0fef1a4f19f3c2e2f4af4c1c104a))
* **deps:** update google.golang.org/genproto digest to fc7c04a ([#598](https://github.com/honeydipper/honeydipper/issues/598)) ([6e5d911](https://github.com/honeydipper/honeydipper/commit/6e5d911d8dcbc7024af2faec9c616585b6169107))
* **deps:** update module cloud.google.com/go/kms to v1.17.1 ([#594](https://github.com/honeydipper/honeydipper/issues/594)) ([6fddfda](https://github.com/honeydipper/honeydipper/commit/6fddfda624dcb3f980f023e2ba959ec26830f4b1))
* **deps:** update module cloud.google.com/go/secretmanager to v1.14.0 ([#599](https://github.com/honeydipper/honeydipper/issues/599)) ([91f9975](https://github.com/honeydipper/honeydipper/commit/91f99750c81860076b8271a1dcc54aa02a6af414))
* **deps:** update module github.com/golang-jwt/jwt/v5 to v5.2.1 ([#595](https://github.com/honeydipper/honeydipper/issues/595)) ([d72e10d](https://github.com/honeydipper/honeydipper/commit/d72e10d7e19fca5a1b4f47f7b1f083d5f3f9b9a9))
* **deps:** update module github.com/stretchr/testify to v1.8.4 ([#585](https://github.com/honeydipper/honeydipper/issues/585)) ([7024463](https://github.com/honeydipper/honeydipper/commit/70244638ecb7c41968aeb354a196de2c7bd3a4bf))
* **deps:** update module golang.org/x/crypto to v0.17.0 [security] ([#583](https://github.com/honeydipper/honeydipper/issues/583)) ([4a20348](https://github.com/honeydipper/honeydipper/commit/4a20348fe92158fc7a2d8f53b4f3984018a3e478))


### Features

* cli job mode ([#601](https://github.com/honeydipper/honeydipper/issues/601)) ([1f5cb44](https://github.com/honeydipper/honeydipper/commit/1f5cb44133074fb3ad2ab396015d3ccecec5ce89)), closes [#590](https://github.com/honeydipper/honeydipper/issues/590)
* **config:** use main as default git branch ([#590](https://github.com/honeydipper/honeydipper/issues/590)) ([7fc8762](https://github.com/honeydipper/honeydipper/commit/7fc876293132d292d97b92ad7f81fd37b08439fd)), closes [#490](https://github.com/honeydipper/honeydipper/issues/490)

## [3.4.1](https://github.com/honeydipper/honeydipper/compare/v3.4.0...v3.4.1) (2023-10-30)


### Bug Fixes

* **deps:** update google.golang.org/genproto digest to 8bfb1ae ([#575](https://github.com/honeydipper/honeydipper/issues/575)) ([d7c8d52](https://github.com/honeydipper/honeydipper/commit/d7c8d525196755003dc07178f4544e45214a9fba))
* **deps:** update module cloud.google.com/go/kms to v1.15.2 ([#576](https://github.com/honeydipper/honeydipper/issues/576)) ([0e8f1c3](https://github.com/honeydipper/honeydipper/commit/0e8f1c34b5c9fa1f96be561814b539e803cf129e))
* not reusing success messages ([#577](https://github.com/honeydipper/honeydipper/issues/577)) ([2b12221](https://github.com/honeydipper/honeydipper/commit/2b122213122363dedaaf540ce28fb569c813d413))

# [3.4.0](https://github.com/honeydipper/honeydipper/compare/v3.3.0...v3.4.0) (2023-09-28)


### Bug Fixes

* **deps:** update golang.org/x/exp digest to 9212866 ([#566](https://github.com/honeydipper/honeydipper/issues/566)) ([1f984b0](https://github.com/honeydipper/honeydipper/commit/1f984b04c35b49b98528c05bc6baf8937d39b59f))
* **deps:** update google.golang.org/genproto digest to 007df8e ([#567](https://github.com/honeydipper/honeydipper/issues/567)) ([87483b2](https://github.com/honeydipper/honeydipper/commit/87483b25d905806da81653995ac0380405e01482))


### Features

* support negated condition in match conditions ([#570](https://github.com/honeydipper/honeydipper/issues/570)) ([efbffcc](https://github.com/honeydipper/honeydipper/commit/efbffcc390cd7de45219baa54dd4e83446156d5f))

# [3.3.0](https://github.com/honeydipper/honeydipper/compare/v3.2.0...v3.3.0) (2023-08-25)


### Bug Fixes

* gcs driver various fixes ([#564](https://github.com/honeydipper/honeydipper/issues/564)) ([ef8c850](https://github.com/honeydipper/honeydipper/commit/ef8c850e7cc0667ff7f0abc2a0adf53399c223cc))


### Features

* gcloud storage writeFile and getAttrs function ([#563](https://github.com/honeydipper/honeydipper/issues/563)) ([34f5008](https://github.com/honeydipper/honeydipper/commit/34f500844bf3d22886b5a4cd23e83714f026b688))

# [3.2.0](https://github.com/honeydipper/honeydipper/compare/v3.1.0...v3.2.0) (2023-08-15)


### Features

* local variables in go templates ([#561](https://github.com/honeydipper/honeydipper/issues/561)) ([e66e2cf](https://github.com/honeydipper/honeydipper/commit/e66e2cf1820714a30ba762226ce9d762bb052512))

# [3.1.0](https://github.com/honeydipper/honeydipper/compare/v3.0.1...v3.1.0) (2023-07-10)


### Bug Fixes

* **deps:** update golang.org/x/exp digest to 97b1e66 ([#556](https://github.com/honeydipper/honeydipper/issues/556)) ([0fc6fc8](https://github.com/honeydipper/honeydipper/commit/0fc6fc8c80786be85907d3b506fa176e7f49e914))
* **deps:** update google.golang.org/genproto digest to ccb25ca ([#557](https://github.com/honeydipper/honeydipper/issues/557)) ([17caafe](https://github.com/honeydipper/honeydipper/commit/17caafecf43d3e481b14fb10b45c729c04645e90))
* run hooks in parallel to avoid interference ([#559](https://github.com/honeydipper/honeydipper/issues/559)) ([13ad15a](https://github.com/honeydipper/honeydipper/commit/13ad15ad67a9f985e5cacad06fa8b636a9d0cfca))


### Features

* detach a child workflow to make it independent ([#558](https://github.com/honeydipper/honeydipper/issues/558)) ([cc6281f](https://github.com/honeydipper/honeydipper/commit/cc6281fa1cc11b55f3490a683c3682a069f2433a))

## [3.0.1](https://github.com/honeydipper/honeydipper/compare/v3.0.0...v3.0.1) (2023-07-01)


### Bug Fixes

* correct web driver token_sources missing headers ([#554](https://github.com/honeydipper/honeydipper/issues/554)) ([be9b756](https://github.com/honeydipper/honeydipper/commit/be9b75626e3f6c53089582f33f7d4aca3328fab4))

# [3.0.0](https://github.com/honeydipper/honeydipper/compare/v2.15.0...v3.0.0) (2023-06-29)


### Features

* support gcp iap authentication for api calls ([#552](https://github.com/honeydipper/honeydipper/issues/552)) ([77324d7](https://github.com/honeydipper/honeydipper/commit/77324d73be704253c3cc1e8560c47e6a6035e5a1))


### BREAKING CHANGES

* casbin policy requires adjustment

The casbin policy needs to change the request to support an additional provider field in requests.

* test: add provider in api tests

* test: add api auth provider to integration test

# [2.15.0](https://github.com/honeydipper/honeydipper/compare/v2.14.0...v2.15.0) (2023-06-20)


### Bug Fixes

* bypass go-git unstaged change with reset ([#549](https://github.com/honeydipper/honeydipper/issues/549)) ([9f3a1ad](https://github.com/honeydipper/honeydipper/commit/9f3a1ad65c0e6963246e08102877ecb8a63ea12b))
* **deps:** update google.golang.org/genproto digest to e85fd2c ([#544](https://github.com/honeydipper/honeydipper/issues/544)) ([e20add8](https://github.com/honeydipper/honeydipper/commit/e20add89f4e8256ef62f80995fc8db4cb97e62d9))
* **deps:** update module github.com/gin-gonic/gin to v1.9.1 [security] ([#543](https://github.com/honeydipper/honeydipper/issues/543)) ([7d7dfe1](https://github.com/honeydipper/honeydipper/commit/7d7dfe13fe3e8edf36edfe58d6d5a4649c1ef9f6))
* **deps:** update module github.com/imdario/mergo to v0.3.16 ([#545](https://github.com/honeydipper/honeydipper/issues/545)) ([3c41e5b](https://github.com/honeydipper/honeydipper/commit/3c41e5be5d8d1d4c61b1644830bd13409b01727e))
* load each context only once ([#546](https://github.com/honeydipper/honeydipper/issues/546)) ([1060096](https://github.com/honeydipper/honeydipper/commit/1060096bdc77a19226d22cffc7ce62d7d95bc6a1))


### Features

* support layered local variable definitions ([#547](https://github.com/honeydipper/honeydipper/issues/547)) ([2df21fc](https://github.com/honeydipper/honeydipper/commit/2df21fcff1f9b251d6d02e6c7817fde9c861c308))
* truncate the labels before displaying in logs ([#550](https://github.com/honeydipper/honeydipper/issues/550)) ([478fc7c](https://github.com/honeydipper/honeydipper/commit/478fc7c4e7e723080b12537ebe16140120a9e607))
* web driver use token sources for authorization ([#548](https://github.com/honeydipper/honeydipper/issues/548)) ([545b736](https://github.com/honeydipper/honeydipper/commit/545b7363eb6bf2155326f97d6e08785873c93c3d))

# [2.14.0](https://github.com/honeydipper/honeydipper/compare/v2.13.0...v2.14.0) (2023-05-30)


### Bug Fixes

* allow config multiple loggerName and projects in gcloud-logging ([#540](https://github.com/honeydipper/honeydipper/issues/540)) ([d06391f](https://github.com/honeydipper/honeydipper/commit/d06391fdbd7a1dfc69f0d0f057b0547b3c68b814))
* **deps:** update google.golang.org/genproto digest to daa745c ([#534](https://github.com/honeydipper/honeydipper/issues/534)) ([86de3d6](https://github.com/honeydipper/honeydipper/commit/86de3d6bb170db76d2d003a781f52be2e61c2192))
* **deps:** update module github.com/gin-gonic/gin to v1.9.0 [security] ([#537](https://github.com/honeydipper/honeydipper/issues/537)) ([dfd9478](https://github.com/honeydipper/honeydipper/commit/dfd947895fdf6b617bddf17805b3e2ea67d0ea4b))


### Features

* add gcloud-logging driver ([#538](https://github.com/honeydipper/honeydipper/issues/538)) ([6b173f6](https://github.com/honeydipper/honeydipper/commit/6b173f6605bcb22dbf12ad8a896db015001911a3))
* kubernetes pvc support ([#539](https://github.com/honeydipper/honeydipper/issues/539)) ([fd092f7](https://github.com/honeydipper/honeydipper/commit/fd092f7dc10ff56603ddaa29923d0387be2904fb))

# [2.13.0](https://github.com/honeydipper/honeydipper/compare/v2.12.1...v2.13.0) (2023-04-07)


### Bug Fixes

* **deps:** update google.golang.org/genproto digest to c38d8f0 ([#530](https://github.com/honeydipper/honeydipper/issues/530)) ([cd0f603](https://github.com/honeydipper/honeydipper/commit/cd0f603b7ef475b178612fdbb8df5ca5c104e02b))
* file reference path correction ([#532](https://github.com/honeydipper/honeydipper/issues/532)) ([f462751](https://github.com/honeydipper/honeydipper/commit/f4627512cea0567d50e90995e637f74d435727c2))


### Features

* loading files matching glob ([#529](https://github.com/honeydipper/honeydipper/issues/529)) ([a7308d8](https://github.com/honeydipper/honeydipper/commit/a7308d8ecaf755707fc9a48cb36ac2f6721eccb2))

## [2.12.1](https://github.com/honeydipper/honeydipper/compare/v2.12.0...v2.12.1) (2023-03-03)


### Bug Fixes

* **core:** workflow export failure protection ([#526](https://github.com/honeydipper/honeydipper/issues/526)) ([183bd6f](https://github.com/honeydipper/honeydipper/commit/183bd6fcde5c4589373fb8cc71dc324b5b8c743c))
* **deps:** update google.golang.org/genproto digest to e74f57a ([#524](https://github.com/honeydipper/honeydipper/issues/524)) ([5667209](https://github.com/honeydipper/honeydipper/commit/5667209f06f57e7d49e2d480b34316c9b65e0198))
* patch method require body in the request ([#522](https://github.com/honeydipper/honeydipper/issues/522)) ([5dbf1e4](https://github.com/honeydipper/honeydipper/commit/5dbf1e4b957dccb0708fff9e989455c6dafe638e))

# [2.12.0](https://github.com/honeydipper/honeydipper/compare/v2.11.1...v2.12.0) (2023-03-01)


### Bug Fixes

* commandHandler protecting the return labels ([#519](https://github.com/honeydipper/honeydipper/issues/519)) ([7e3fa4d](https://github.com/honeydipper/honeydipper/commit/7e3fa4df2ee32f5c5c9524b5692a1a1ded7c5179))


### Features

* allow export on error ([#521](https://github.com/honeydipper/honeydipper/issues/521)) ([614bc0f](https://github.com/honeydipper/honeydipper/commit/614bc0fb8d50d8a7729479447fa5af4a935d26b6))

## [2.11.1](https://github.com/honeydipper/honeydipper/compare/v2.11.0...v2.11.1) (2023-02-25)


### Bug Fixes

* **core:** refresh repo should load correct ref ([#517](https://github.com/honeydipper/honeydipper/issues/517)) ([717bdd0](https://github.com/honeydipper/honeydipper/commit/717bdd0eb66e27a35e2db9c93c8763e29b959bfb))

# [2.11.0](https://github.com/honeydipper/honeydipper/compare/v2.10.0...v2.11.0) (2023-02-23)


### Bug Fixes

* **core:** resume_token should be unique among cluster members ([#512](https://github.com/honeydipper/honeydipper/issues/512)) ([8f47a15](https://github.com/honeydipper/honeydipper/commit/8f47a158d08982e24808f940f2bf86f3b78a5eb0))
* **deps:** update sprig functions to v3 ([#516](https://github.com/honeydipper/honeydipper/issues/516)) ([3a4716f](https://github.com/honeydipper/honeydipper/commit/3a4716f6c9c64b59d4699f6e7fbb0543afcc377f))


### Features

* **core:** allow override a repo with a directory ([#511](https://github.com/honeydipper/honeydipper/issues/511)) ([0ccb251](https://github.com/honeydipper/honeydipper/commit/0ccb2513b952b8576081069d701bb08e0bbf7913))
* **core:** allow return typed data from go template ([#514](https://github.com/honeydipper/honeydipper/issues/514)) ([f70b923](https://github.com/honeydipper/honeydipper/commit/f70b923188ca4cf184d783876b64b3f18aea7e74))

# [2.10.0](https://github.com/honeydipper/honeydipper/compare/v2.9.0...v2.10.0) (2023-02-09)


### Bug Fixes

* **deps:** update golang.org/x/crypto digest to bc19a97 ([#487](https://github.com/honeydipper/honeydipper/issues/487)) ([6f9b0c5](https://github.com/honeydipper/honeydipper/commit/6f9b0c559d9b613142b1a1823b76d49822842f8a))
* **deps:** update golang.org/x/oauth2 digest to 0ebed06 ([#488](https://github.com/honeydipper/honeydipper/issues/488)) ([b2196af](https://github.com/honeydipper/honeydipper/commit/b2196af015466d739e25346040def48d523ff2ef))
* **deps:** update google.golang.org/genproto digest to 008b390 ([#504](https://github.com/honeydipper/honeydipper/issues/504)) ([a6b4a18](https://github.com/honeydipper/honeydipper/commit/a6b4a1835b9ce35b67f4c35aa2ab6e45356e9416))
* **deps:** update google.golang.org/genproto digest to 28d6b97 ([#495](https://github.com/honeydipper/honeydipper/issues/495)) ([873c1dd](https://github.com/honeydipper/honeydipper/commit/873c1ddaa0eec2063b8a159cd7fa29b2906256a6))
* **deps:** update module github.com/imdario/mergo to v0.3.13 ([#498](https://github.com/honeydipper/honeydipper/issues/498)) ([8626323](https://github.com/honeydipper/honeydipper/commit/86263236e685dbfdccaa3a15ae78ed06a39ec8cb))


### Features

* **core:** graceful shutdown by draining drivers ([#489](https://github.com/honeydipper/honeydipper/issues/489)) ([f8c00a1](https://github.com/honeydipper/honeydipper/commit/f8c00a1365924b45df6bf7216942421bdcafbed7))
* env var interpolation and load-time interpolation ([#501](https://github.com/honeydipper/honeydipper/issues/501)) ([9b1c659](https://github.com/honeydipper/honeydipper/commit/9b1c659606a33d9634be473edc03a3bcc9192146)), closes [#496](https://github.com/honeydipper/honeydipper/issues/496)
* use redis as a cache storage through RPC ([#497](https://github.com/honeydipper/honeydipper/issues/497)) ([2b0a588](https://github.com/honeydipper/honeydipper/commit/2b0a588c5469f83b719c210d54fa4de26e8bba0e))

# [2.9.0](https://github.com/honeydipper/honeydipper/compare/v2.8.1...v2.9.0) (2022-08-03)


### Bug Fixes

* add RequestHeaderTimeout to http servers ([#483](https://github.com/honeydipper/honeydipper/issues/483)) ([b19d596](https://github.com/honeydipper/honeydipper/commit/b19d596d7ea6df75e551f39a66b22c0940ba2001))
* API default timeout use store writeTimeout ([#481](https://github.com/honeydipper/honeydipper/issues/481)) ([9069d7b](https://github.com/honeydipper/honeydipper/commit/9069d7befaa93adba939c2d6840342cdc3981cb4))
* **deps:** update golang.org/x/crypto digest to 0559593 ([#477](https://github.com/honeydipper/honeydipper/issues/477)) ([5ddb5e4](https://github.com/honeydipper/honeydipper/commit/5ddb5e432a17fd701c23c3817b05ed66276b072b))
* **deps:** update golang.org/x/oauth2 digest to 2104d58 ([#468](https://github.com/honeydipper/honeydipper/issues/468)) ([37539da](https://github.com/honeydipper/honeydipper/commit/37539da6a4819757dad0771a7acf7d8a06068905))
* **deps:** update golang.org/x/term digest to 065cf7b ([#478](https://github.com/honeydipper/honeydipper/issues/478)) ([9af1806](https://github.com/honeydipper/honeydipper/commit/9af18066930fea684723bed232de49763c29d512))
* **deps:** update google.golang.org/genproto digest to 590a5ac ([#479](https://github.com/honeydipper/honeydipper/issues/479)) ([db534e2](https://github.com/honeydipper/honeydipper/commit/db534e20088b0b6380caa0c8cf3db8fca4dc8aae))
* **deps:** update kubernetes packages to v0.24.2 ([#471](https://github.com/honeydipper/honeydipper/issues/471)) ([ede6311](https://github.com/honeydipper/honeydipper/commit/ede63111cc22147093163fcd72b97230d4695ae7))
* **deps:** update module github.com/go-redis/redis/v8 to v8.11.5 ([#476](https://github.com/honeydipper/honeydipper/issues/476)) ([7b92f30](https://github.com/honeydipper/honeydipper/commit/7b92f30f788e24583e5c4280bfd23448b6880750))
* **deps:** update module github.com/gogf/gf to v1.16.9 ([#480](https://github.com/honeydipper/honeydipper/issues/480)) ([07e95a1](https://github.com/honeydipper/honeydipper/commit/07e95a1a684398b04c9f8936cfbf1b0bb99597da))


### Features

* allow repo specific ssh keys ([#484](https://github.com/honeydipper/honeydipper/issues/484)) ([559b59f](https://github.com/honeydipper/honeydipper/commit/559b59fbbad7ff06465182d2e35b60e87cd7b896))

## [2.8.1](https://github.com/honeydipper/honeydipper/compare/v2.8.0...v2.8.1) (2022-03-30)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 4570a08 ([#453](https://github.com/honeydipper/honeydipper/issues/453)) ([2264769](https://github.com/honeydipper/honeydipper/commit/2264769c0c4c91f08b3e84b2af748f45b7d2ac51))
* **deps:** update golang.org/x/crypto commit hash to 8634188 ([#457](https://github.com/honeydipper/honeydipper/issues/457)) ([3354c64](https://github.com/honeydipper/honeydipper/commit/3354c640ed6f18c07f11b502750258858b95f9fc))
* **deps:** update golang.org/x/crypto commit hash to e495a2d ([#455](https://github.com/honeydipper/honeydipper/issues/455)) ([d310ee9](https://github.com/honeydipper/honeydipper/commit/d310ee96ce8331d9eb64e2be70ec29e3cf55726f))
* **deps:** update golang.org/x/oauth2 commit hash to d3ed0bb ([#454](https://github.com/honeydipper/honeydipper/issues/454)) ([591cbae](https://github.com/honeydipper/honeydipper/commit/591cbaece7df53d0b8efb6420c64bc798c9b543f))
* **deps:** update golang.org/x/oauth2 commit hash to ee48083 ([#462](https://github.com/honeydipper/honeydipper/issues/462)) ([e474648](https://github.com/honeydipper/honeydipper/commit/e4746485cc0c78a9e18636d7e445cdc691a56c17))
* **deps:** update golang.org/x/term commit hash to 03fcf44 ([#456](https://github.com/honeydipper/honeydipper/issues/456)) ([0b44b58](https://github.com/honeydipper/honeydipper/commit/0b44b584f0842d76f2f438175a1f1b7c0aaf0824))
* **deps:** update google.golang.org/genproto commit hash to 325a892 ([#458](https://github.com/honeydipper/honeydipper/issues/458)) ([d4acc76](https://github.com/honeydipper/honeydipper/commit/d4acc76199ff9d20355690948ba904bc4b63cea4))
* **deps:** update module github.com/gin-gonic/gin to v1.7.4 ([#448](https://github.com/honeydipper/honeydipper/issues/448)) ([8435181](https://github.com/honeydipper/honeydipper/commit/8435181e76b7f726f7f954e53c7405ea9445530b))
* **deps:** update module k8s.io/api to v0.22.4 ([#451](https://github.com/honeydipper/honeydipper/issues/451)) ([310006e](https://github.com/honeydipper/honeydipper/commit/310006edc3ec363ca7b31c2128b74ac0a6bbb9dc))

# [2.8.0](https://github.com/honeydipper/honeydipper/compare/v2.7.0...v2.8.0) (2021-10-01)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 0a44fdf ([#428](https://github.com/honeydipper/honeydipper/issues/428)) ([2961189](https://github.com/honeydipper/honeydipper/commit/2961189334e2515ecaa7151c043f3e5691b5213c))
* **deps:** update golang.org/x/crypto commit hash to 32db794 ([#433](https://github.com/honeydipper/honeydipper/issues/433)) ([e8b0462](https://github.com/honeydipper/honeydipper/commit/e8b046273774041934e08ef24cdffc132047f909))
* **deps:** update golang.org/x/crypto commit hash to 5ff15b2 ([#421](https://github.com/honeydipper/honeydipper/issues/421)) ([423896a](https://github.com/honeydipper/honeydipper/commit/423896a94542cfdcae011ba8f9816a727c5e8469))
* **deps:** update golang.org/x/oauth2 commit hash to 2bc19b1 ([#434](https://github.com/honeydipper/honeydipper/issues/434)) ([ee1db57](https://github.com/honeydipper/honeydipper/commit/ee1db57aa22856a62f6f53731b6e4b9a5b9dcd70))
* **deps:** update golang.org/x/oauth2 commit hash to a41e5a7 ([#422](https://github.com/honeydipper/honeydipper/issues/422)) ([bf1b15e](https://github.com/honeydipper/honeydipper/commit/bf1b15ef6cbf34afd9debceda6b74e9261ca6dc9))
* **deps:** update golang.org/x/term commit hash to 6886f2d ([#423](https://github.com/honeydipper/honeydipper/issues/423)) ([4d55069](https://github.com/honeydipper/honeydipper/commit/4d550696d0eaa40627b43ab03e71af501edd1b65))
* **deps:** update google.golang.org/genproto commit hash to 8c882eb ([#424](https://github.com/honeydipper/honeydipper/issues/424)) ([7011bd0](https://github.com/honeydipper/honeydipper/commit/7011bd006665c21e99b7a42e45f1e7ce5518492b))
* **deps:** update google.golang.org/genproto commit hash to d08c68a ([#435](https://github.com/honeydipper/honeydipper/issues/435)) ([738334e](https://github.com/honeydipper/honeydipper/commit/738334e54dc178a26b67e5dbecd12667e8e8a3fe))
* **deps:** update google.golang.org/genproto commit hash to e15ff19 ([#429](https://github.com/honeydipper/honeydipper/issues/429)) ([db1c248](https://github.com/honeydipper/honeydipper/commit/db1c248b428e102085e886b9509a15b32896cce3))
* **deps:** update module github.com/imdario/mergo to v0.3.12 ([#425](https://github.com/honeydipper/honeydipper/issues/425)) ([972d57f](https://github.com/honeydipper/honeydipper/commit/972d57fcdc62485125b17a5918b4294f418108ac))
* **drivers:** web and webhook recognize more json content types ([#441](https://github.com/honeydipper/honeydipper/issues/441)) ([924e99c](https://github.com/honeydipper/honeydipper/commit/924e99cae67dc875e535691684de1d38d3eefd4c))


### Features

* **drivers:** create k8s job from cronjob spec ([#440](https://github.com/honeydipper/honeydipper/issues/440)) ([e3982fc](https://github.com/honeydipper/honeydipper/commit/e3982fca3dbb7a713b3d55b80909a0bee75c8ed0))

# [2.7.0](https://github.com/honeydipper/honeydipper/compare/v2.6.0...v2.7.0) (2021-06-30)


### Features

* **drivers:** datadog-emitter allow functions ([#418](https://github.com/honeydipper/honeydipper/issues/418)) ([7329f6b](https://github.com/honeydipper/honeydipper/commit/7329f6b34edb1a6609651562b20e3b1aedba5fe9))
* **drivers:** webhook hmac support secret list ([#417](https://github.com/honeydipper/honeydipper/issues/417)) ([312a350](https://github.com/honeydipper/honeydipper/commit/312a350128368ce1abbf6b544e27afd8c3bbaf37))

# [2.6.0](https://github.com/honeydipper/honeydipper/compare/v2.5.0...v2.6.0) (2021-06-04)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to c07d793 ([#411](https://github.com/honeydipper/honeydipper/issues/411)) ([282dd70](https://github.com/honeydipper/honeydipper/commit/282dd7093373e345e93891bb7cc0d69bbc81b304))
* **deps:** update golang.org/x/oauth2 commit hash to f6687ab ([#412](https://github.com/honeydipper/honeydipper/issues/412)) ([4f309c7](https://github.com/honeydipper/honeydipper/commit/4f309c77d3e7ef141621742493f578deab4a1a73))
* **deps:** update google.golang.org/genproto commit hash to 58e84a5 ([#413](https://github.com/honeydipper/honeydipper/issues/413)) ([8bbaecd](https://github.com/honeydipper/honeydipper/commit/8bbaecd9f2ad965612946ca1e3e3c65f4d70ba0e))


### Features

* **drivers:** web request support multiple value query variables ([#415](https://github.com/honeydipper/honeydipper/issues/415)) ([b5c26a7](https://github.com/honeydipper/honeydipper/commit/b5c26a7da3291de22259896fc9afb4f560f536ba))

# [2.5.0](https://github.com/honeydipper/honeydipper/compare/v2.4.0...v2.5.0) (2021-06-01)


### Features

* **drivers:** webhook verify signature using hmac ([#407](https://github.com/honeydipper/honeydipper/issues/407)) ([462ade6](https://github.com/honeydipper/honeydipper/commit/462ade6c47967856704c025ac197308dfa6d2a2c))

# [2.4.0](https://github.com/honeydipper/honeydipper/compare/v2.3.1...v2.4.0) (2021-05-11)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 38f3c27 ([#398](https://github.com/honeydipper/honeydipper/issues/398)) ([8ca52a8](https://github.com/honeydipper/honeydipper/commit/8ca52a825db4ca2a3ce5435a3b4faa55213b229c))
* **deps:** update golang.org/x/oauth2 commit hash to 81ed05c ([#399](https://github.com/honeydipper/honeydipper/issues/399)) ([4142a60](https://github.com/honeydipper/honeydipper/commit/4142a60ae3a1e8c61a9449fefab5bb9b405958b8))
* **deps:** update golang.org/x/term commit hash to a79de54 ([#400](https://github.com/honeydipper/honeydipper/issues/400)) ([fdab939](https://github.com/honeydipper/honeydipper/commit/fdab93977836e9c0ceaa4f5d62f1055d1ad52fed))
* **deps:** update google.golang.org/genproto commit hash to 3b2ad6c ([#401](https://github.com/honeydipper/honeydipper/issues/401)) ([6766ffc](https://github.com/honeydipper/honeydipper/commit/6766ffc2c4055cc27d3865cfc6a03debe1048eae))
* **internal:** re-order git SSH auth options ([#403](https://github.com/honeydipper/honeydipper/issues/403)) ([69bd070](https://github.com/honeydipper/honeydipper/commit/69bd0706894c7ef7dbea0edf3653d832ffd56f61))


### Features

* **drivers:** gcloud dataflow supports name pattern matching ([#404](https://github.com/honeydipper/honeydipper/issues/404)) ([5c33c5a](https://github.com/honeydipper/honeydipper/commit/5c33c5a8cb912c329a63f770a13b4ad2551c866d))
* **drivers:** gcloud-secret support shorthand key name ([6c5eaa2](https://github.com/honeydipper/honeydipper/commit/6c5eaa22c770eca264c67890bc49d4386e3a1583))
* **drivers:** load redis option from env variable ([#402](https://github.com/honeydipper/honeydipper/issues/402)) ([2a4b34d](https://github.com/honeydipper/honeydipper/commit/2a4b34d37e8cbff3ff7e06612bc1bb67bc3e405c))
* **operator:** support deferred decryption and lookup ([f8abc83](https://github.com/honeydipper/honeydipper/commit/f8abc83babde7a67ca630621867037cc49f6194d))
* **workflow:** keep track of start and completion time ([#397](https://github.com/honeydipper/honeydipper/issues/397)) ([5cc4571](https://github.com/honeydipper/honeydipper/commit/5cc4571066d355dc59a3fccac5013b248ce36f31))

## [2.3.1](https://github.com/honeydipper/honeydipper/compare/v2.3.0...v2.3.1) (2021-04-14)


### Bug Fixes

* **config:** contaminated dataset crashes during reload ([#393](https://github.com/honeydipper/honeydipper/issues/393)) ([f483352](https://github.com/honeydipper/honeydipper/commit/f48335214cb7c4bbd852f29c5d42db78b6753777))
* **deps:** update golang.org/x/crypto commit hash to 0c34fe9 ([#390](https://github.com/honeydipper/honeydipper/issues/390)) ([6594a88](https://github.com/honeydipper/honeydipper/commit/6594a884a6e5766af3337df1186be5b96af35cf1))
* **deps:** update golang.org/x/oauth2 commit hash to 2e8d934 ([#391](https://github.com/honeydipper/honeydipper/issues/391)) ([e1eabba](https://github.com/honeydipper/honeydipper/commit/e1eabba990cb199ed834c68538fbdaff16c5b5e1))

# [2.3.0](https://github.com/honeydipper/honeydipper/compare/v2.2.0...v2.3.0) (2021-03-04)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 5ea612d ([#383](https://github.com/honeydipper/honeydipper/issues/383)) ([8197ee0](https://github.com/honeydipper/honeydipper/commit/8197ee05ed3c0b45ef44029a68bbd416bc1145f0))
* **deps:** update golang.org/x/oauth2 commit hash to 0101308 ([#373](https://github.com/honeydipper/honeydipper/issues/373)) ([9d00d72](https://github.com/honeydipper/honeydipper/commit/9d00d7205a6c5a97c15963c680ce5afc4ffb57f4))
* **deps:** update golang.org/x/oauth2 commit hash to 9bb9049 ([#384](https://github.com/honeydipper/honeydipper/issues/384)) ([ada3958](https://github.com/honeydipper/honeydipper/commit/ada3958a2a6663b69130d7b360893436bdc26854))
* **deps:** update golang.org/x/term commit hash to 2321bbc ([#374](https://github.com/honeydipper/honeydipper/issues/374)) ([9e5ecee](https://github.com/honeydipper/honeydipper/commit/9e5ecee1ddda2a99aa4f3ddacadd8d12e7ebe193))
* **deps:** update golang.org/x/term commit hash to 6a3ed07 ([#385](https://github.com/honeydipper/honeydipper/issues/385)) ([925ddc1](https://github.com/honeydipper/honeydipper/commit/925ddc1fe5a7c954578c44a0c09b13efbed7afa1))
* **deps:** update google.golang.org/genproto commit hash to 9728d6b ([#386](https://github.com/honeydipper/honeydipper/issues/386)) ([d02675d](https://github.com/honeydipper/honeydipper/commit/d02675d1dab89671693fb5e73c98bcceeeacdfe4))
* **deps:** update google.golang.org/genproto commit hash to bba0dbe ([#375](https://github.com/honeydipper/honeydipper/issues/375)) ([e87d7e9](https://github.com/honeydipper/honeydipper/commit/e87d7e9a018fbb37270f9e71321dc6ff37f9f0f7))
* **dipper:** driver ReadySignal chan initialization ([#381](https://github.com/honeydipper/honeydipper/issues/381)) ([e413893](https://github.com/honeydipper/honeydipper/commit/e413893a8d6420a77f10020903d51641ea9d9bbf))


### Features

* **drivers:** fetch secrets from google secret manager ([#382](https://github.com/honeydipper/honeydipper/issues/382)) ([0bf283f](https://github.com/honeydipper/honeydipper/commit/0bf283fdddc225dbd03f80366da0aa4cbbe94a72))

# [2.2.0](https://github.com/honeydipper/honeydipper/compare/v2.1.4...v2.2.0) (2021-01-25)


### Bug Fixes

* **drivers:** redis drivers avoid printing password ([8391b01](https://github.com/honeydipper/honeydipper/commit/8391b0139c4ad988413b287938ffdf6c30075e14))
* **services:** api service discovery using staged dataset ([#369](https://github.com/honeydipper/honeydipper/issues/369)) ([a6a2615](https://github.com/honeydipper/honeydipper/commit/a6a26156f8b27ba5ccb19a0aa6e7052dad288f83))


### Features

* **dipper:** helper function to check truthy value ([78dc32a](https://github.com/honeydipper/honeydipper/commit/78dc32ad36a1a8ae673b839a05bae685d66618e4))
* **dipper:** support deferred decryption in drivers ([37619a6](https://github.com/honeydipper/honeydipper/commit/37619a6d626b05793ac4f9110581e26fca44b80b))
* **drivers:** redis drivers support TLS ([48c979c](https://github.com/honeydipper/honeydipper/commit/48c979cb4c446c6be01e58bb24f9cf5379b10853))

## [2.1.4](https://github.com/honeydipper/honeydipper/compare/v2.1.3...v2.1.4) (2021-01-04)


### Bug Fixes

* **cmd:** configcheck should use staged dataset ([52dc879](https://github.com/honeydipper/honeydipper/commit/52dc879ee04b80b8b11cc418df895e795b5ec11f)), closes [#358](https://github.com/honeydipper/honeydipper/issues/358)
* **cmd:** docgen should use staged dataset ([6472801](https://github.com/honeydipper/honeydipper/commit/6472801e3bf761e5dfacc284d6663f7567087f4f)), closes [#358](https://github.com/honeydipper/honeydipper/issues/358)
* **config:** safely wrap WaitGroup.Wait ([9f3ebe4](https://github.com/honeydipper/honeydipper/commit/9f3ebe40de57e3313414a8b309e891480d27b07c))
* **deps:** update golang.org/x/crypto commit hash to eec23a3 ([#366](https://github.com/honeydipper/honeydipper/issues/366)) ([4ee8d5c](https://github.com/honeydipper/honeydipper/commit/4ee8d5c488caf954da4bd16cb9fc003b2a7b237a))
* **deps:** update golang.org/x/oauth2 commit hash to 08078c5 ([#367](https://github.com/honeydipper/honeydipper/issues/367)) ([8caa327](https://github.com/honeydipper/honeydipper/commit/8caa327cdbbff0b460f562579f5915ec9e260908))

## [2.1.3](https://github.com/honeydipper/honeydipper/compare/v2.1.2...v2.1.3) (2020-12-29)


### Bug Fixes

* **config:** avoid concurrent map write in config ([#358](https://github.com/honeydipper/honeydipper/issues/358)) ([d53b0e3](https://github.com/honeydipper/honeydipper/commit/d53b0e3a4e0d449655f816134d536e32e8b59ba7))
* **deps:** update golang.org/x/crypto commit hash to 9e8e0b3 ([#333](https://github.com/honeydipper/honeydipper/issues/333)) ([1417aa8](https://github.com/honeydipper/honeydipper/commit/1417aa841c440c4f567f49b41d92e639b84e4482))
* **deps:** update golang.org/x/oauth2 commit hash to 0b49973 ([#345](https://github.com/honeydipper/honeydipper/issues/345)) ([40df4a1](https://github.com/honeydipper/honeydipper/commit/40df4a154cf95d535423b088fe942f04c279c0a0))
* **deps:** update google.golang.org/genproto commit hash to 06b3db8 ([#346](https://github.com/honeydipper/honeydipper/issues/346)) ([5918d0f](https://github.com/honeydipper/honeydipper/commit/5918d0f56cdca8058ae5689ad4d91f206f3ad898))
* **deps:** update google.golang.org/genproto commit hash to 2e45c02 ([#334](https://github.com/honeydipper/honeydipper/issues/334)) ([13a2cb4](https://github.com/honeydipper/honeydipper/commit/13a2cb4613838b7d049ccdc8973a6ccbc200b143))
* **deps:** update to aurora v3 ([#341](https://github.com/honeydipper/honeydipper/issues/341)) ([363dc85](https://github.com/honeydipper/honeydipper/commit/363dc8528c1fb249ee3448bef9fd13a70e5ac199))
* **internal:** remove wfdata interpolation ([#355](https://github.com/honeydipper/honeydipper/issues/355)) ([6bc1b5d](https://github.com/honeydipper/honeydipper/commit/6bc1b5d41fab6fad289738b0de3ff608661e1464))
* **logging:** switch to golang.org/x/term ([#350](https://github.com/honeydipper/honeydipper/issues/350)) ([81c9043](https://github.com/honeydipper/honeydipper/commit/81c9043afba3c820ffd4f096c05e1f4c91b20691))
* **workflow:** avoid concurrent map write in mergeContext ([#357](https://github.com/honeydipper/honeydipper/issues/357)) ([a7b8380](https://github.com/honeydipper/honeydipper/commit/a7b8380c7382bdf49ec5c085e0fc93cea848c66f))

## [2.1.2](https://github.com/honeydipper/honeydipper/compare/v2.1.1...v2.1.2) (2020-10-30)


### Bug Fixes

* **drivers:** k8s getJobLog misplaced Close statement ([#331](https://github.com/honeydipper/honeydipper/issues/331)) ([4f493b5](https://github.com/honeydipper/honeydipper/commit/4f493b5fb8b6f16c83fc6e1d4a3ae49bce1a8bd3))

## [2.1.1](https://github.com/honeydipper/honeydipper/compare/v2.1.0...v2.1.1) (2020-10-24)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 7f63de1 ([#321](https://github.com/honeydipper/honeydipper/issues/321)) ([5fe6d9e](https://github.com/honeydipper/honeydipper/commit/5fe6d9ee30309863844d508b1871b4a9e44790ee))
* **deps:** update google.golang.org/genproto commit hash to 3860012 ([#322](https://github.com/honeydipper/honeydipper/issues/322)) ([e292546](https://github.com/honeydipper/honeydipper/commit/e2925461102024260bd8b9d6a49df84da59f163e))
* **deps:** update module casbin/casbin/v2 to v2.13.1 ([#326](https://github.com/honeydipper/honeydipper/issues/326)) ([bb1f2a7](https://github.com/honeydipper/honeydipper/commit/bb1f2a76007b93e8c4dae12ef9803046746f9ae1))
* **deps:** update module cloud.google.com/go to v0.68.0 ([#327](https://github.com/honeydipper/honeydipper/issues/327)) ([48b20ec](https://github.com/honeydipper/honeydipper/commit/48b20ec06241aae3b059834dfae4c8893f1eba62))
* **pkg:** driver api_timeout under data ([5a088c9](https://github.com/honeydipper/honeydipper/commit/5a088c9b8a549e3c856e71d1e4961c695d2188c9))
* **workflow:** revert simplify export ([#329](https://github.com/honeydipper/honeydipper/issues/329)) ([4a5e195](https://github.com/honeydipper/honeydipper/commit/4a5e19571438c7756405f6eac4c5e31795e066c8))

# [2.1.0](https://github.com/honeydipper/honeydipper/compare/v2.0.0...v2.1.0) (2020-09-15)


### Bug Fixes

* **api:** timing issue in tests ([3297f17](https://github.com/honeydipper/honeydipper/commit/3297f17eb5f927fd4a8850d658d0edddfb5b6bf5))
* **deps:** update golang.org/x/crypto commit hash to 5c72a88 ([#314](https://github.com/honeydipper/honeydipper/issues/314)) ([7ce1fa4](https://github.com/honeydipper/honeydipper/commit/7ce1fa4b44c3eb9e38e804e30070c9774b4aae4a))
* **deps:** update golang.org/x/oauth2 commit hash to 5d25da1 ([#315](https://github.com/honeydipper/honeydipper/issues/315)) ([8cc3ca8](https://github.com/honeydipper/honeydipper/commit/8cc3ca8d60ed58b1e1321a303d387718b0a3b8a6))
* **deps:** update google.golang.org/genproto commit hash to 0bd0a95 ([#316](https://github.com/honeydipper/honeydipper/issues/316)) ([b5133eb](https://github.com/honeydipper/honeydipper/commit/b5133ebbe203a4f8511b75afbfd7eb91bec3beef))
* **drivers:** auth-simple handling authorization header ([baf5894](https://github.com/honeydipper/honeydipper/commit/baf589426bea7d803137873347d734c9dc2022ef))


### Features

* **api:** healthcheck and configurable prefix ([f694ce1](https://github.com/honeydipper/honeydipper/commit/f694ce16a1bff08c1e5c33c5260efc9d6ae71e16))
* **api:** use casbin authorization library ([a85976e](https://github.com/honeydipper/honeydipper/commit/a85976eb0d06aac12a7ec9d4e6711d16ddbfcab5))
* **configcheck:** run tests against auth rules in configcheck ([dde3d4c](https://github.com/honeydipper/honeydipper/commit/dde3d4cbdeec915c49e08ed727194d93045d16e7))

# [2.0.0](https://github.com/honeydipper/honeydipper/compare/v1.7.0...v2.0.0) (2020-08-29)


### Bug Fixes

* **api:** deny API access when no ACL matches ([1ea0bea](https://github.com/honeydipper/honeydipper/commit/1ea0beaa022a72480fe270491ad2ba4c083d4ecf))
* **dipper:** rpc handles returns with nil payload ([860c5cf](https://github.com/honeydipper/honeydipper/commit/860c5cff076b6d882e809dc52f559694c6cf829c))
* **drivers:** auth-simple uses bcrypt on password and token ([e830c6e](https://github.com/honeydipper/honeydipper/commit/e830c6e43979cb766018716783858c3e3f79a8be))
* **drivers:** webhook only return uuid when requested ([0f08057](https://github.com/honeydipper/honeydipper/commit/0f080574e60e4130eecbec513f7c918359461f3f))


### Code Refactoring

* **drivers:** redispubsub payload structuring ([6c572e3](https://github.com/honeydipper/honeydipper/commit/6c572e3dd5509ea941fc098fbcaa8968a84286d1))


### Features

* **api:** a simple authorization framework ([c4e2a56](https://github.com/honeydipper/honeydipper/commit/c4e2a56a276db6d4eb1d5ee18730ec6391ecea06))
* **drivers:** adding lock driver ([fe2b254](https://github.com/honeydipper/honeydipper/commit/fe2b254af7d2a739c86dec7c80eb6b49b0169b03))
* **drivers:** broadcast allow targetting to a service using labels ([067b491](https://github.com/honeydipper/honeydipper/commit/067b49141ac4a74ae428f6a19d2d9f839c0cc597))
* **drivers:** broadcast support RPC in addition to Command ([1a01f92](https://github.com/honeydipper/honeydipper/commit/1a01f928b59619cee778439d2039ba69a94a5d67))
* **drivers:** make broadcast channel configurable ([0d6f70e](https://github.com/honeydipper/honeydipper/commit/0d6f70e9ee2203b8a86d27b35c4bbda1b37da019))
* **drivers:** redispubsub/redisqueue adding from label ([cd80a25](https://github.com/honeydipper/honeydipper/commit/cd80a250504c370346afef8dfc76c7068861b005))
* **drivers:** redisqueue to support future api service ([0ea6e20](https://github.com/honeydipper/honeydipper/commit/0ea6e2009848534b85d42b2e5600f8b58aba60f1))
* **services:** add eventAdd api to receiver service ([7c11081](https://github.com/honeydipper/honeydipper/commit/7c1108158e93104d65e1a8c7b6a8ca21feff4343))
* **services:** add eventList api in engine service ([a859993](https://github.com/honeydipper/honeydipper/commit/a8599933ced0d4db825d0cd8527137c10ccf1030))
* **services:** add eventWait api in engine service ([82af37b](https://github.com/honeydipper/honeydipper/commit/82af37bf566fc53ad61f80084a0877741e24307d))
* **services:** establishing a new service for API ([1a38cdd](https://github.com/honeydipper/honeydipper/commit/1a38cdd70db8ecb6c6228157a04e76e91e4feb37))
* **services:** use a uuid to track event/workflow lifecycle ([cd3b0a4](https://github.com/honeydipper/honeydipper/commit/cd3b0a4d02860238ffff03a6e5e9f0fbd33a3f20))
* **workflow:** exposes workflow session information ([df551bb](https://github.com/honeydipper/honeydipper/commit/df551bb695958d36f616d975c67a8311564f731c))


### Styles

* **drivers:** use backupOpID instead of backupOpId in google-spanner ([246f5f2](https://github.com/honeydipper/honeydipper/commit/246f5f24e6e7e5fd3aeb7b70ed59711d66fb9215))


### BREAKING CHANGES

* **drivers:** needs to update the spanner native backup related system/workflows.
* **drivers:** the payload structure is slightly changed so that the three piece of information
can be fetched from the right place. The existing `reload` function in `honeydipper-config-essentials`
requires corresponding change to function. So is the slack `resume_session`.

# [1.7.0](https://github.com/honeydipper/honeydipper/compare/v1.6.1...v1.7.0) (2020-08-03)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 123391f ([#301](https://github.com/honeydipper/honeydipper/issues/301)) ([c99884a](https://github.com/honeydipper/honeydipper/commit/c99884a4bc1129727d0315154fd9f301fd7d83f4))
* **deps:** update google.golang.org/genproto commit hash to 8145dea ([#302](https://github.com/honeydipper/honeydipper/issues/302)) ([754306e](https://github.com/honeydipper/honeydipper/commit/754306e3e898c82fb5835b4d6bd3bb1deeae7c7f))
* **deps:** update module cloud.google.com/go to v0.62.0 ([#305](https://github.com/honeydipper/honeydipper/issues/305)) ([d782200](https://github.com/honeydipper/honeydipper/commit/d782200ce55008c7ed7edc85ac031dc96f4308b0))
* **deps:** update module golang/mock to v1.4.4 ([#306](https://github.com/honeydipper/honeydipper/issues/306)) ([5f03653](https://github.com/honeydipper/honeydipper/commit/5f036536b33a3910cc1c69fcf63e24b0dcf8c7f9))


### Features

* **dipper:** helper error handling function Must ([408796b](https://github.com/honeydipper/honeydipper/commit/408796bebb77241deddfd4ca001b05c3162ca5a0))
* **drivers:** spanner and k8s createJob allows retry ([5d391b1](https://github.com/honeydipper/honeydipper/commit/5d391b1039a2f0ed3274dbcb4d3ddeafc037e554))

## [1.6.1](https://github.com/honeydipper/honeydipper/compare/v1.6.0...v1.6.1) (2020-07-20)


### Bug Fixes

* **dipper:** map modifier handle nil value gracefully ([#295](https://github.com/honeydipper/honeydipper/issues/295)) ([c4710c1](https://github.com/honeydipper/honeydipper/commit/c4710c1371931d7162ce050f045ad902e9328ec0))
* **drivers:** watch API requries calling get and re-authentication ([#297](https://github.com/honeydipper/honeydipper/issues/297)) ([8b73614](https://github.com/honeydipper/honeydipper/commit/8b7361472713b68035d605f9489d3df6b89d7c07)), closes [#296](https://github.com/honeydipper/honeydipper/issues/296)

# [1.6.0](https://github.com/honeydipper/honeydipper/compare/v1.5.2...v1.6.0) (2020-07-07)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 75b2880 ([#284](https://github.com/honeydipper/honeydipper/issues/284)) ([0a0f811](https://github.com/honeydipper/honeydipper/commit/0a0f8111890388d4612815d9d6c1ac0681fb3888))
* **deps:** update google.golang.org/genproto commit hash to 0750642 ([#285](https://github.com/honeydipper/honeydipper/issues/285)) ([6852a5c](https://github.com/honeydipper/honeydipper/commit/6852a5ceb9ef32d86cee904e009e9e3b775486b4))
* **deps:** update module cloud.google.com/go to v0.60.0 ([#287](https://github.com/honeydipper/honeydipper/issues/287)) ([795b207](https://github.com/honeydipper/honeydipper/commit/795b2075df6eb7e8611d2c1f7638c6ff2a1dbd30))
* **deps:** update module datadog/datadog-go to v3.7.2 ([#288](https://github.com/honeydipper/honeydipper/issues/288)) ([60a5b92](https://github.com/honeydipper/honeydipper/commit/60a5b9292ccbbbd8bc43a54bcab0a355da02f86f))


### Features

* **drivers:** spanner driver for taking native backups ([#283](https://github.com/honeydipper/honeydipper/issues/283)) ([07700e4](https://github.com/honeydipper/honeydipper/commit/07700e498b664a7eb13c004bb126c31d6102c14f))

## [1.5.2](https://github.com/honeydipper/honeydipper/compare/v1.5.1...v1.5.2) (2020-06-25)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 70a84ac ([#273](https://github.com/honeydipper/honeydipper/issues/273)) ([3e033d2](https://github.com/honeydipper/honeydipper/commit/3e033d2256d0c690843d9a6d47a75b02cb89defe))
* **deps:** update google.golang.org/genproto commit hash to d0ee0c3 ([#274](https://github.com/honeydipper/honeydipper/issues/274)) ([cff10ec](https://github.com/honeydipper/honeydipper/commit/cff10ec242f98a2c8c07c8777bd75cfa580f3645))
* **deps:** update module cloud.google.com/go to v0.58.0 ([#276](https://github.com/honeydipper/honeydipper/issues/276)) ([8df54ba](https://github.com/honeydipper/honeydipper/commit/8df54ba7f9a5944ffd14b40a8e824b2dd9a7dc7d))
* **deps:** update module mitchellh/mapstructure to v1.3.2 ([#277](https://github.com/honeydipper/honeydipper/issues/277)) ([7c6cf05](https://github.com/honeydipper/honeydipper/commit/7c6cf057cde5887a7bb729e05e4a9fcdc5c0c3c9))
* **deps:** update module stretchr/testify to v1.6.1 ([#278](https://github.com/honeydipper/honeydipper/issues/278)) ([89cc46b](https://github.com/honeydipper/honeydipper/commit/89cc46b01e521c43d92cd74b8f5715ee5ca48988))
* **k8s:** cascading deletion of pods when deleting jobs ([#281](https://github.com/honeydipper/honeydipper/issues/281)) ([b3d3609](https://github.com/honeydipper/honeydipper/commit/b3d3609c285a99e8e4505c4e79a20c6800369325))
* **service:** reload driver after driver crashing ([#272](https://github.com/honeydipper/honeydipper/issues/272)) ([898af7b](https://github.com/honeydipper/honeydipper/commit/898af7b47ef43bd636157e3a55146560f46ed3e4))

## [1.5.1](https://github.com/honeydipper/honeydipper/compare/v1.5.0...v1.5.1) (2020-06-04)


### Bug Fixes

* **config:** function export should arrange parent subsystem data properly ([cced8c8](https://github.com/honeydipper/honeydipper/commit/cced8c86d158ab8f60504cc15d0d2ee9d26fcb2a))
* **configcheck:** exit non-zero on context errors ([#256](https://github.com/honeydipper/honeydipper/issues/256)) ([ca7cd2b](https://github.com/honeydipper/honeydipper/commit/ca7cd2bbd08ce309d6bf6893cde6b02b79b2b532)), closes [#255](https://github.com/honeydipper/honeydipper/issues/255)
* **deps:** update google.golang.org/genproto commit hash to 0b04860 ([f1153a7](https://github.com/honeydipper/honeydipper/commit/f1153a71b0585897bfd850bbe6222cc5db6f4a6d))
* **deps:** update module go-errors/errors to v1.1.1 ([74b5698](https://github.com/honeydipper/honeydipper/commit/74b56986af5eff47cb01f68184b856fad1ba77fd))
* **deps:** update module go-redis/redis to v6.15.8 ([#261](https://github.com/honeydipper/honeydipper/issues/261)) ([cf8b7e0](https://github.com/honeydipper/honeydipper/commit/cf8b7e096f25b594d391f6e383a9dad53c00418b))
* **deps:** update module google.golang.org/api to v0.26.0 ([e331e99](https://github.com/honeydipper/honeydipper/commit/e331e99af2cc14323f7ee17059af938ffdad791f))
* **deps:** update module k8s.io/client-go to v0.18.3 ([56ced48](https://github.com/honeydipper/honeydipper/commit/56ced48623731883c920a038823497548d5dd904))
* **deps:** update module mitchellh/mapstructure to v1.3.1 ([#266](https://github.com/honeydipper/honeydipper/issues/266)) ([97c4196](https://github.com/honeydipper/honeydipper/commit/97c4196ada9fd586b613ccac0a1cba38a554159a))

# [1.5.0](https://github.com/honeydipper/honeydipper/compare/v1.4.0...v1.5.0) (2020-05-29)


### Bug Fixes

* **drivers:** driver panic due to closed channel during retry ([#251](https://github.com/honeydipper/honeydipper/issues/251)) ([8814c43](https://github.com/honeydipper/honeydipper/commit/8814c43433389c7f830a44e90b713776a34ce699))


### Features

* **config:** allow using params and sysData in function export ([790d8d4](https://github.com/honeydipper/honeydipper/commit/790d8d45db82f22720756a4d6c4b508e377dcc5d))
* **interpolation:** allow indirect access in dollar interpolation ([b21d091](https://github.com/honeydipper/honeydipper/commit/b21d091805c61e8bbb88890aa1a5e7eb7e150723))

# [1.4.0](https://github.com/honeydipper/honeydipper/compare/v1.3.0...v1.4.0) (2020-05-20)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 06a226f ([#239](https://github.com/honeydipper/honeydipper/issues/239)) ([9184c54](https://github.com/honeydipper/honeydipper/commit/9184c54e25848240b78e57b8f9979c3c0e882833))
* **deps:** update module cloud.google.com/go to v0.57.0 ([#242](https://github.com/honeydipper/honeydipper/issues/242)) ([a5544f2](https://github.com/honeydipper/honeydipper/commit/a5544f21f7073796fce4007ac69c2b1d8780f3ab))
* **deps:** update module google.golang.org/api to v0.24.0 ([#243](https://github.com/honeydipper/honeydipper/issues/243)) ([79e0195](https://github.com/honeydipper/honeydipper/commit/79e019507481f777ead31eebbc59d3e2223815ec))
* **deps:** update module yaml to v2.3.0 ([#244](https://github.com/honeydipper/honeydipper/issues/244)) ([4cfa36f](https://github.com/honeydipper/honeydipper/commit/4cfa36fe4522193fccab3df8922e2b0c354367fa))
* **workflow:** truthy condition check should recognize <no value> ([#247](https://github.com/honeydipper/honeydipper/issues/247)) ([8fd5465](https://github.com/honeydipper/honeydipper/commit/8fd5465b8a4e20d6831350ad37b6f8651b7aef11))


### Features

* **k8s:** add a function for deleting jobs ([#246](https://github.com/honeydipper/honeydipper/issues/246)) ([9f85164](https://github.com/honeydipper/honeydipper/commit/9f851648307a279ecdf03d0cf9d509afe8d6d227))

# [1.3.0](https://github.com/honeydipper/honeydipper/compare/v1.2.0...v1.3.0) (2020-05-14)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to 4b2356b ([#203](https://github.com/honeydipper/honeydipper/issues/203)) ([d60d37b](https://github.com/honeydipper/honeydipper/commit/d60d37bdf5cb8391d26b75b6c84fe742a8babc6f))
* **deps:** update google.golang.org/genproto commit hash to b979b6f ([#204](https://github.com/honeydipper/honeydipper/issues/204)) ([3104fd5](https://github.com/honeydipper/honeydipper/commit/3104fd577d2b9b324abb36d6d9abb733175bea69))
* **deps:** update module google.golang.org/api to v0.23.0 ([#233](https://github.com/honeydipper/honeydipper/issues/233)) ([c58af2d](https://github.com/honeydipper/honeydipper/commit/c58af2d0591733a09ee941b2a9e31728c674040b))
* **deps:** update module k8s.io/api to v0.18.2 ([#217](https://github.com/honeydipper/honeydipper/issues/217)) ([49649bc](https://github.com/honeydipper/honeydipper/commit/49649bccb3dbcc700fd6448669744cdba35d5170))
* **deps:** update module k8s.io/apimachinery to v0.18.2 ([#218](https://github.com/honeydipper/honeydipper/issues/218)) ([d9fee30](https://github.com/honeydipper/honeydipper/commit/d9fee303d44e2929185a125c9fb6db2392ce334f))
* **deps:** update module k8s.io/client-go to v0.18.2 ([3afec34](https://github.com/honeydipper/honeydipper/commit/3afec346e510c195bbb334fcd1a80ddc262e75f2))
* **deps:** update module mitchellh/mapstructure to v1.3.0 ([#231](https://github.com/honeydipper/honeydipper/issues/231)) ([be20a60](https://github.com/honeydipper/honeydipper/commit/be20a60f7b60fc3dcca5e33119f0a455b19a6bf4))
* **k8s:** support client-go 0.18.2 with contexts supporting timeout ([d74bcc9](https://github.com/honeydipper/honeydipper/commit/d74bcc9712d49db4b360879c91f4150fd33911aa))


### Features

* **cmd:** enforcing variable timeout for functions ([#237](https://github.com/honeydipper/honeydipper/issues/237)) ([89bb23a](https://github.com/honeydipper/honeydipper/commit/89bb23a0e2419f59029efe9df0f1a5288286aae3))

# [1.2.0](https://github.com/honeydipper/honeydipper/compare/v1.1.0...v1.2.0) (2020-05-04)


### Bug Fixes

* **deps:** update module cloud.google.com/go to v0.56.0 ([#207](https://github.com/honeydipper/honeydipper/issues/207)) ([ffd1819](https://github.com/honeydipper/honeydipper/commit/ffd181998860ff070387a326d59fd28cad254d48))
* **deps:** update module datadog/datadog-go to v3.6.0 ([#208](https://github.com/honeydipper/honeydipper/issues/208)) ([9290ca8](https://github.com/honeydipper/honeydipper/commit/9290ca8adbd3e4a84060bbe7648e36622ead4e5e))
* **deps:** update module datadog/datadog-go to v3.7.1 ([#230](https://github.com/honeydipper/honeydipper/issues/230)) ([7921b95](https://github.com/honeydipper/honeydipper/commit/7921b957c14b0c34b2698c2f29a86ccfea07c1fb))
* **deps:** update module go-errors/errors to v1.0.2 ([#220](https://github.com/honeydipper/honeydipper/issues/220)) ([46ef3f3](https://github.com/honeydipper/honeydipper/commit/46ef3f3d35ff3e37459f0b005206fdcf19d947fb))
* **deps:** update module google.golang.org/api to v0.22.0 ([#215](https://github.com/honeydipper/honeydipper/issues/215)) ([ac027e4](https://github.com/honeydipper/honeydipper/commit/ac027e4dfe30a7ff442a24b695e5c3822fde01a3))
* **deps:** update module masterminds/sprig/v3 to v3.1.0 ([#221](https://github.com/honeydipper/honeydipper/issues/221)) ([175ccf3](https://github.com/honeydipper/honeydipper/commit/175ccf354cf1f809e48d469f00053a205ac63241))


### Features

* **drivers:** recycle k8s deployment by name ([#228](https://github.com/honeydipper/honeydipper/issues/228)) ([04d2836](https://github.com/honeydipper/honeydipper/commit/04d2836c418110586aed910b54d377d8b50963d8))

# [1.1.0](https://github.com/honeydipper/honeydipper/compare/v1.0.8...v1.1.0) (2020-04-01)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to f7b0055 ([#196](https://github.com/honeydipper/honeydipper/issues/196)) ([9657ae1](https://github.com/honeydipper/honeydipper/commit/9657ae1077afb1d80312c99e7661fa38310ae80b))
* **deps:** update google.golang.org/genproto commit hash to 303a050 ([#192](https://github.com/honeydipper/honeydipper/issues/192)) ([1cdc375](https://github.com/honeydipper/honeydipper/commit/1cdc3750f2747f5e2cc613cba73dded33021b564))
* **deps:** update module cloud.google.com/go to v0.54.0 ([#200](https://github.com/honeydipper/honeydipper/issues/200)) ([b596509](https://github.com/honeydipper/honeydipper/commit/b5965091d39b459b7dde6deb33cf45356ed47836))
* **deps:** update module golang/mock to v1.4.3 ([#201](https://github.com/honeydipper/honeydipper/issues/201)) ([a4b3827](https://github.com/honeydipper/honeydipper/commit/a4b3827d1502227af596fc38bb3d61da1c07754a))
* **deps:** update module google.golang.org/api to v0.20.0 ([#198](https://github.com/honeydipper/honeydipper/issues/198)) ([2690a75](https://github.com/honeydipper/honeydipper/commit/2690a753e3b4ed3343de4701a4f8657bce7e46a9))
* **deps:** update module k8s.io/api to v0.17.4 ([#182](https://github.com/honeydipper/honeydipper/issues/182)) ([52d3c2e](https://github.com/honeydipper/honeydipper/commit/52d3c2e9ed3120bf7852ae45a0b49851b1136f81))
* stop passing backoff_ms, retry labels ([#202](https://github.com/honeydipper/honeydipper/issues/202)) ([f0fd933](https://github.com/honeydipper/honeydipper/commit/f0fd933379fb5215f01792b04b61f87e4b68102d))
* **deps:** update module mitchellh/mapstructure to v1.2.2 ([#210](https://github.com/honeydipper/honeydipper/issues/210)) ([9b2e9f9](https://github.com/honeydipper/honeydipper/commit/9b2e9f9729ca31ccf9728549466fbca18eb07619))
* **deps:** update module stretchr/testify to v1.5.1 ([#195](https://github.com/honeydipper/honeydipper/issues/195)) ([4de822a](https://github.com/honeydipper/honeydipper/commit/4de822acebed77d8d9b5962ba9d70fd18b89b1e3))


### Features

* add contexts configcheck ([#212](https://github.com/honeydipper/honeydipper/issues/212)) ([f920916](https://github.com/honeydipper/honeydipper/commit/f920916169956f27a068e6902946fb09a653bef2))

## [1.0.8](https://github.com/honeydipper/honeydipper/compare/v1.0.7...v1.0.8) (2020-02-20)


### Bug Fixes

* **config:** regex patterns are not parsed in unless_match ([b6a12d6](https://github.com/honeydipper/honeydipper/commit/b6a12d69f978f7a8d40933373f42202525945d05))
* **deps:** update golang.org/x/crypto commit hash to 1d94cc7 ([#178](https://github.com/honeydipper/honeydipper/issues/178)) ([1864239](https://github.com/honeydipper/honeydipper/commit/186423956d99d51cf508dceb96de1d17681745c8))
* **deps:** update google.golang.org/genproto commit hash to 66ed5ce ([#177](https://github.com/honeydipper/honeydipper/issues/177)) ([acc5867](https://github.com/honeydipper/honeydipper/commit/acc58677868c846be7b156440c2e90515c6e5c41))
* **deps:** update module cloud.google.com/go to v0.53.0 ([#186](https://github.com/honeydipper/honeydipper/issues/186)) ([ddcac5f](https://github.com/honeydipper/honeydipper/commit/ddcac5f2fafa26b9f2f92d76f8117eb21163f21e))
* **deps:** update module stretchr/testify to v1.5.0 ([#193](https://github.com/honeydipper/honeydipper/issues/193)) ([b23c5e4](https://github.com/honeydipper/honeydipper/commit/b23c5e48d1ead1f2973367b5069a21d2045d36c5))
* **dipper:** comma lost in default value for dollar interpolation ([36521bb](https://github.com/honeydipper/honeydipper/commit/36521bb8f92f6ddb6a942252873b31901d04fd87))

## [1.0.7](https://github.com/honeydipper/honeydipper/compare/v1.0.6...v1.0.7) (2020-02-12)


### Bug Fixes

* **deps:** update golang.org/x/crypto commit hash to a0c6ece ([4bb817e](https://github.com/honeydipper/honeydipper/commit/4bb817e3af48978ea23b3e125d181a877d6bb591))
* **deps:** update google.golang.org/genproto commit hash to a86caf9 ([#176](https://github.com/honeydipper/honeydipper/issues/176)) ([d40f737](https://github.com/honeydipper/honeydipper/commit/d40f7372b42281890d09b8bd35ea489dbd262b8a))
* **deps:** update google.golang.org/genproto commit hash to f68cdc7 ([#172](https://github.com/honeydipper/honeydipper/issues/172)) ([b77b9ad](https://github.com/honeydipper/honeydipper/commit/b77b9ad0835ed02bbbb32012bfacfaccf526806d))
* **deps:** update google.golang.org/genproto commit hash to fa8e72b ([#173](https://github.com/honeydipper/honeydipper/issues/173)) ([5ba8465](https://github.com/honeydipper/honeydipper/commit/5ba84650cc384664646b91c15d8fbfadc9b3301e))
* **deps:** update module go-redis/redis to v6.15.7 ([#171](https://github.com/honeydipper/honeydipper/issues/171)) ([a26c9bf](https://github.com/honeydipper/honeydipper/commit/a26c9bf67f6de68f5ea2d9d4b1862aa2144f60bd))
* **deps:** update module google.golang.org/api to v0.17.0 ([#175](https://github.com/honeydipper/honeydipper/issues/175)) ([dd0599d](https://github.com/honeydipper/honeydipper/commit/dd0599d07af6849e1eceda508389101b6f24df6c))

## [1.0.6](https://github.com/honeydipper/honeydipper/compare/v1.0.5...v1.0.6) (2020-01-31)


### Bug Fixes

* **deps:** update golang docker tag to v1.13.7 ([#153](https://github.com/honeydipper/honeydipper/issues/153)) ([14e2e57](https://github.com/honeydipper/honeydipper/commit/14e2e57bd5a81cac9fc6d0e22dad2eb7f3e45fdd))
* **deps:** update module cloud.google.com/go to v0.52.0 ([#160](https://github.com/honeydipper/honeydipper/issues/160)) ([94bd945](https://github.com/honeydipper/honeydipper/commit/94bd945e2f9761e33523a3d5a2b061eede7888d4))
* **deps:** update module datadog/datadog-go to v3.4.0 ([#161](https://github.com/honeydipper/honeydipper/issues/161)) ([4d98a8e](https://github.com/honeydipper/honeydipper/commit/4d98a8e31e3037371708fcddcc735bebe0b8a2cc))
* **deps:** update module k8s.io/client-go to v0.17.2 ([99fd2cd](https://github.com/honeydipper/honeydipper/commit/99fd2cd4acb54c61356ccce68091ed442e8809cd))
* **deps:** update module masterminds/sprig to v3 ([5170197](https://github.com/honeydipper/honeydipper/commit/51701979127d44bf08aa9e1fd5fc106db7712278))
* **deps:** update module yaml to v2.2.8 ([627ecc7](https://github.com/honeydipper/honeydipper/commit/627ecc716e6461788d637a1a1e59bbfa20045349))

## [1.0.5](https://github.com/honeydipper/honeydipper/compare/v1.0.4...v1.0.5) (2020-01-29)


### Bug Fixes

* **deps:** update github.com/logrusorgru/aurora commit hash to e9ef32d ([#146](https://github.com/honeydipper/honeydipper/issues/146)) ([10cda5d](https://github.com/honeydipper/honeydipper/commit/10cda5da55fd57550f002a1815f0a048ff6ec12b))
* **deps:** update golang.org/x/crypto commit hash to 69ecbb4 ([#147](https://github.com/honeydipper/honeydipper/issues/147)) ([201860d](https://github.com/honeydipper/honeydipper/commit/201860d95f6dedd5ba6cadd8714e3630e0519022))
* **deps:** update google.golang.org/genproto commit hash to 58ce757 ([#150](https://github.com/honeydipper/honeydipper/issues/150)) ([600634a](https://github.com/honeydipper/honeydipper/commit/600634aca1bb6a905df24e0428c99d5ff641da53))
* **deps:** Update to go 1.13.6 ([#156](https://github.com/honeydipper/honeydipper/issues/156)) ([66be36d](https://github.com/honeydipper/honeydipper/commit/66be36dede537bf00c3ab2b0e4c71ebcadff5497))

## [1.0.4](https://github.com/honeydipper/honeydipper/compare/v1.0.3...v1.0.4) (2020-01-28)


### Bug Fixes

* safe reading function for sessions map ([94cb267](https://github.com/honeydipper/honeydipper/commit/94cb26746572d88c6ef7d9f7ecdcca0eca2b381f))

## [1.0.3](https://github.com/honeydipper/honeydipper/compare/v1.0.2...v1.0.3) (2020-01-10)


### Bug Fixes

* pubsub driver supporting version 1.0.0 and later collapsedEvents format ([0bed990](https://github.com/honeydipper/honeydipper/commit/0bed990))
* uses concurent safe reading function while reading Result map ([b9444ee](https://github.com/honeydipper/honeydipper/commit/b9444ee))

## [1.0.2](https://github.com/honeydipper/honeydipper/compare/v1.0.1...v1.0.2) (2019-12-05)


### Bug Fixes

* error in completion hook causing run away workflow sessions ([498be3b](https://github.com/honeydipper/honeydipper/commit/498be3b))

## [1.0.1](https://github.com/honeydipper/honeydipper/compare/v1.0.0...v1.0.1) (2019-11-16)


### Features

* function and rawAction retry and backoff ([fef6635](https://github.com/honeydipper/honeydipper/commit/fef6635))

## [1.0.0](https://github.com/honeydipper/honeydipper/compare/v0.2.0...v1.0.0) (2019-10-21)



### Features

* DipperCL initial implementation ([ab92ab2](https://github.com/honeydipper/honeydipper/commit/ab92ab2))

## [0.2.0](https://github.com/honeydipper/honeydipper/compare/v0.1.10...v0.2.0) (2019-08-22)



### Bug Fixes

* dep ensure ran out of memory when building @ codefresh.io ([21f0bf1](https://github.com/honeydipper/honeydipper/commit/21f0bf1))
* **security:** upgrade to 1.12.9 to fix CVE-2019-9512 CVE-2019-9514 ([937a1fb](https://github.com/honeydipper/honeydipper/commit/937a1fb))


### Features

* **driver:** gcloud-pubsub driver for subscriber ([512da53](https://github.com/honeydipper/honeydipper/commit/512da53))

## [0.1.10](https://github.com/honeydipper/honeydipper/compare/v0.1.9...v0.1.10) (2019-06-20)



### Bug Fixes

* short circuit the /hz/alive healthcheck for now ([6e7e727](https://github.com/honeydipper/honeydipper/commit/6e7e727))
* using map in multiple threads with locking ([5f7da77](https://github.com/honeydipper/honeydipper/commit/5f7da77))


### Features

* dataflow driver enhancements ([453e2c3](https://github.com/honeydipper/honeydipper/commit/453e2c3))

## [0.1.9](https://github.com/honeydipper/honeydipper/compare/v0.1.8...v0.1.9) (2019-05-08)



### Bug Fixes

* **test:** guarding against strange http errors in webhook test ([dc63ebe](https://github.com/honeydipper/honeydipper/commit/dc63ebe))


### Features

* **dataflow:** added updateJob function for draining and cancelling job ([d6e2ffb](https://github.com/honeydipper/honeydipper/commit/d6e2ffb))
* **engine:** function/event data export into context ([b3af017](https://github.com/honeydipper/honeydipper/commit/b3af017))
* **driver:** extract host and remoteAddr from eventData (#97) ([cf8777c](https://github.com/honeydipper/honeydipper/commit/cf8777c)), closes [#85](https://github.com/honeydipper/honeydipper/issues/85) [#97](https://github.com/honeydipper/honeydipper/issues/97) [#85](https://github.com/honeydipper/honeydipper/issues/85)
* **suspend:** suspended session can resume with a timeout ([56062e3](https://github.com/honeydipper/honeydipper/commit/56062e3))
* **configcheck:** a few check related to wfdata ([3d679b3](https://github.com/honeydipper/honeydipper/commit/3d679b3))
* **web:** #92 json Payload use interface ([99edb5c](https://github.com/honeydipper/honeydipper/commit/99edb5c)), closes [#92](https://github.com/honeydipper/honeydipper/issues/92)
* **configcheck:** run config check before publish configurations (#90) ([4fc5799](https://github.com/honeydipper/honeydipper/commit/4fc5799)), closes [#90](https://github.com/honeydipper/honeydipper/issues/90)

## [0.1.8](https://github.com/honeydipper/honeydipper/compare/v0.1.7...v0.1.8) (2019-03-20)


### Bug Fixes

* **build:** Install git in Docker image ([fe04266](https://github.com/honeydipper/honeydipper/commit/fe042663483e65d38e08030140c8dfedc958750f)), closes [#84](https://github.com/honeydipper/honeydipper/issues/84)
* **daemon:** Don’t panic when Payload is nil ([#88](https://github.com/honeydipper/honeydipper/issues/88)) ([42cf9ca](https://github.com/honeydipper/honeydipper/commit/42cf9ca0ab80c3d2dfd96405bb915a69910c18e8))

## [0.1.7](https://github.com/honeydipper/honeydipper/compare/v0.1.6...v0.1.7) (2019-03-01)



### Bug Fixes

* **config:** trigger conditions not merging when extending systems ([23d26a0](https://github.com/honeydipper/honeydipper/commit/23d26a082ed4fac82c180b097602a798b9d26d87))
* **integration:** using relative path to find test config in code repo ([df699d6](https://github.com/honeydipper/honeydipper/commit/df699d66821be023d322f84c70ee954170d93513))
* **kubernetes:** recycling deployment by killing the correct replicaset ([81d59bf](https://github.com/honeydipper/honeydipper/commit/81d59bfb70e342c31c0aee8f4f8379a31f024e61))
* **service:** adding recover for all child go routines ([ebeda62](https://github.com/honeydipper/honeydipper/commit/ebeda62b1b65f301fad008937d6cfc939a40b5e0))
* **service:** detecting emitter absence and crashing and test ([264005a](https://github.com/honeydipper/honeydipper/commit/264005aa7c0fd3da5933557875659b66da50a55c))
* **service:** issue [#34](https://github.com/honeydipper/honeydipper/issues/34) daemon crashing when operator call undefined driver ([9ca189b](https://github.com/honeydipper/honeydipper/commit/9ca189b78faa3230955b3e80a6133f0779e444d4))
* Update path in Dockerfile ([363e78b](https://github.com/honeydipper/honeydipper/commit/363e78b)), closes [#54](https://github.com/honeydipper/honeydipper/issues/54)


### Features

* **helm:** make service nodePort customizable ([1c2adc4](https://github.com/honeydipper/honeydipper/commit/1c2adc48712c334ce2edbb9c88bd7a434a4ae055))
* Re-enable goimports (#48) ([5fc4b5d](https://github.com/honeydipper/honeydipper/commit/5fc4b5d)), closes [#48](https://github.com/honeydipper/honeydipper/issues/48)

## [0.1.6](https://github.com/honeydipper/honeydipper/compare/v0.1.5...v0.1.6) (2019-02-15)



### Bug Fixes

* Remove unnecessary syscall functions ([7bbf454](https://github.com/honeydipper/honeydipper/commit/7bbf454))


### Features

* adding listJob and getJob to dataflow driver ([7e9a052](https://github.com/honeydipper/honeydipper/commit/7e9a052))
* allow using default client in gcp ([d2ffdd8](https://github.com/honeydipper/honeydipper/commit/d2ffdd8))

## [0.1.5](https://github.com/honeydipper/honeydipper/compare/v0.1.4...v0.1.5) (2019-02-10)

## [0.1.4](https://github.com/honeydipper/honeydipper/compare/v0.1.3...v0.1.4) (2019-02-05)



### Bug Fixes

* preventing log stream from being closed involuntarily ([ff81eaa](https://github.com/honeydipper/honeydipper/commit/ff81eaa))
* wfdata could be nil in operator ([44bfb3a](https://github.com/honeydipper/honeydipper/commit/44bfb3a))

### Features

* suspend resume workflow (#27) ([9b3e180](https://github.com/honeydipper/honeydipper/commit/9b3e180)), closes [#27](https://github.com/honeydipper/honeydipper/issues/27)
* receiver side interpolation (#26) ([f55d53f](https://github.com/honeydipper/honeydipper/commit/f55d53f)), closes [#26](https://github.com/honeydipper/honeydipper/issues/26)
* A few enhancements (#25) ([1c9050d](https://github.com/honeydipper/honeydipper/commit/1c9050d)), closes [#25](https://github.com/honeydipper/honeydipper/issues/25)
* adding gcloud-dataflow driver ([88daedf](https://github.com/honeydipper/honeydipper/commit/88daedf))

## [0.1.3](https://github.com/honeydipper/honeydipper/compare/v0.1.2...v0.1.3) (2019-01-25)



### Features

* kubernetes to create job, wait for job, get job log ([2110507](https://github.com/honeydipper/honeydipper/commit/2110507))
* enhance logging, workflow fail gracefully, kubernetes job log retrieval ([3b0fa09](https://github.com/honeydipper/honeydipper/commit/3b0fa09))
* interpolate twice so we can use sysData in wfdata ([d931d04](https://github.com/honeydipper/honeydipper/commit/d931d04))

## [0.1.2](https://github.com/honeydipper/honeydipper/compare/v0.1.1...v0.1.2) (2019-01-14)



### Features

* adding a redis pub/sub driver ([6f1c9a8](https://github.com/honeydipper/honeydipper/commit/6f1c9a8))
* adding feature/driverRuntime state ([8571512](https://github.com/honeydipper/honeydipper/commit/8571512))
* datadog emitter driver ([2b0d6ca](https://github.com/honeydipper/honeydipper/commit/2b0d6ca))

## [0.1.1](https://github.com/honeydipper/honeydipper/compare/v0.1.0...v0.1.1) (2019-01-02)



### Features

* more flexible workflow interpolation to support looping ([1aab5cf](https://github.com/honeydipper/honeydipper/commit/1aab5cf))
* reducing the kms call frequency ([6e48144](https://github.com/honeydipper/honeydipper/commit/6e48144))
* adding spec to chart to allow ndots config ([f03b151](https://github.com/honeydipper/honeydipper/commit/f03b151))

## 0.1.0 (2018-12-19)



### Features

* now receivers send collapsedEvents as standard ([b064bc7](https://github.com/honeydipper/honeydipper/commit/b064bc7))
* moving condition checking into dipper ([c3d4f40](https://github.com/honeydipper/honeydipper/commit/c3d4f40))
* moving driver data processing from driver to dipper stub so it can be shared ([aba8359](https://github.com/honeydipper/honeydipper/commit/aba8359))
* simplifying logging and rpc ([3bee6e0](https://github.com/honeydipper/honeydipper/commit/3bee6e0))
* moving logging to dipper ([421b038](https://github.com/honeydipper/honeydipper/commit/421b038))
* moving driver data processing such as decryption to service ([0dbd88b](https://github.com/honeydipper/honeydipper/commit/0dbd88b))
* correcting the path for the source code ([91a9a55](https://github.com/honeydipper/honeydipper/commit/91a9a55))
* logging improvement ([8bf8ffb](https://github.com/honeydipper/honeydipper/commit/8bf8ffb))
* major refactor for rpc and workflow management ([dadfc0c](https://github.com/honeydipper/honeydipper/commit/dadfc0c))
* poc rc1 ([b36965a](https://github.com/honeydipper/honeydipper/commit/b36965a))
