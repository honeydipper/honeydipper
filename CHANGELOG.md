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
