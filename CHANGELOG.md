# Changelog

## [2.1.1](https://github.com/bluegardenproject/tracks/compare/v2.1.0...v2.1.1) (2026-10-06)


### Bug Fixes

* **agents:** start dev servers only when the user asks ([9174ff6](https://github.com/bluegardenproject/tracks/commit/9174ff686567822487538889a184b963ff3eb49c))
* **hooks:** let Claude's sandbox reach the daemon socket ([1c0fd88](https://github.com/bluegardenproject/tracks/commit/1c0fd889e87351eaf325b8bcd12b3142f620e01c))

## [2.1.0](https://github.com/bluegardenproject/tracks/compare/v2.0.1...v2.1.0) (2026-10-06)


### Features

* **agents:** the work and review prompts know the dev servers ([6d8d4bd](https://github.com/bluegardenproject/tracks/commit/6d8d4bdc373e896427f180b0f85a65b030688337))
* **proxy:** the daemon forwards output ports to dev servers ([988d5e9](https://github.com/bluegardenproject/tracks/commit/988d5e990a910a45f7d7d3a67b662c8f3ea354be))
* **repos:** setup command and dev servers per repo ([8cf6bdf](https://github.com/bluegardenproject/tracks/commit/8cf6bdf146241a375653310153ba7230a0de56cc))
* **tracks:** find what listens in a track's panes ([d441f5b](https://github.com/bluegardenproject/tracks/commit/d441f5bc9e96291a8d351d958bc1b8092e48055c))
* **tracks:** run each repo's setup in a pane, and copy .env files ([1cc7c20](https://github.com/bluegardenproject/tracks/commit/1cc7c20a9687e7a8dca835765870c95576e7fa4e))
* **tracks:** server and setup errors on Station ([8c1b824](https://github.com/bluegardenproject/tracks/commit/8c1b824fa180c6771069a37572cf02671e35ce66))
* **tracks:** tracks up, down and logs run dev servers in panes ([140a338](https://github.com/bluegardenproject/tracks/commit/140a338b08bb1147f71a5f3c58ff5b44cea7ab51))
* **tracksview:** the Proxy tab ([0c0cdee](https://github.com/bluegardenproject/tracks/commit/0c0cdee5586e29b70be7084fcbaca0d4312edc01))


### Bug Fixes

* **tracks:** stopping a dev server doesn't wait on zombies ([3fc9515](https://github.com/bluegardenproject/tracks/commit/3fc9515437e5f8cbc475179d6254fdc43f4561b9))
* **trackwin:** a setup that finishes before its pane is labelled isn't an error ([fbadd27](https://github.com/bluegardenproject/tracks/commit/fbadd279e1193b11a25b066eacac3b890e863117))

## [2.0.1](https://github.com/bluegardenproject/tracks/compare/v2.0.0...v2.0.1) (2026-10-02)


### Bug Fixes

* **install:** replace the binary atomically in make install ([11b768c](https://github.com/bluegardenproject/tracks/commit/11b768cfb53f15e9a344bf52e02c34b340e6d284))

## [2.0.0](https://github.com/bluegardenproject/tracks/compare/v1.3.1...v2.0.0) (2026-10-02)


### ⚠ BREAKING CHANGES

* v1 is gone. Finish v1's tracks and stop v1 before installing; its worktrees stay in ~/.local/state/tracks/worktrees but no binary resumes them. Dev servers and the proxy (`tracks up`, `down`, `services`, `url`) are not in 2.0.0.

### Features

* Tracks v2 replaces v1 ([a8a2602](https://github.com/bluegardenproject/tracks/commit/a8a2602f03b23b43aaa465a1a1d09f31140f5d65))


### Miscellaneous

* **v2:** AGENTS.md describes one app ([3228bfb](https://github.com/bluegardenproject/tracks/commit/3228bfbd84b2e5b8436647838079567e1e0dc511))
* **v2:** build, CI and scripts for tracks without v1 ([ec229b3](https://github.com/bluegardenproject/tracks/commit/ec229b33c0200dc0af00d3f000e26052f6337486))
* **v2:** CI and releases build with Go 1.27 ([3bd1c2c](https://github.com/bluegardenproject/tracks/commit/3bd1c2c230c532b44250715f3c47e8de725916d2))
* **v2:** delete v1; tracks runs v2 ([88ea517](https://github.com/bluegardenproject/tracks/commit/88ea517af2098d902359b87d6aac7c004b62e2ac))
* **v2:** golden files keep the prompts, texts and helpers ([d8ec973](https://github.com/bluegardenproject/tracks/commit/d8ec97380ed1aafa287a5cb979fbed95223f559a))
* **v2:** install.sh --local installs a binary from a checkout ([dc8d9fe](https://github.com/bluegardenproject/tracks/commit/dc8d9fe913675a294205d2d65c87d83d417d7307))
* **v2:** internal/v2 moves up to internal ([b760a4b](https://github.com/bluegardenproject/tracks/commit/b760a4b746da109db2085582a99cbba90d94c639))
* **v2:** plain names and paths; no --new-app ([e3a5f49](https://github.com/bluegardenproject/tracks/commit/e3a5f4926bd4ced1ff94b87a4a8d789c062f01b5))
* **v2:** the golden files replace the comparisons with v1 ([c59aebf](https://github.com/bluegardenproject/tracks/commit/c59aebf4fe5eb4b2efb9810fc24dc621fc66d86f))
* **v2:** the README describes v2, with a glossary ([42ac566](https://github.com/bluegardenproject/tracks/commit/42ac5661117f4d18d6da27cc74f611cc81252fd4))
* **v2:** the ROADMAP lists v2's open items ([1136117](https://github.com/bluegardenproject/tracks/commit/11361178e1aa235e674c29e84ecaef96dbf95c6e))
* **v2:** tracks update ([2d810bb](https://github.com/bluegardenproject/tracks/commit/2d810bb94070e32e03d23ad15cb0d6d6671932b7))
* **v2:** tracks version ([6edd470](https://github.com/bluegardenproject/tracks/commit/6edd470985524369faa58d69eeaf64a036e32ed5))
* **v2:** usage owns its Usage type ([d1a7a16](https://github.com/bluegardenproject/tracks/commit/d1a7a16cf44e6ab7b013c79b84b517e5a6e0c8e7))

## [1.3.1](https://github.com/bluegardenproject/tracks/compare/v1.3.0...v1.3.1) (2026-10-01)


### Miscellaneous

* **v2:** a hidden hook command that reports what the agent does ([8a48df0](https://github.com/bluegardenproject/tracks/commit/8a48df0d8798587bf090006826b8b41b91fc898d))
* **v2:** a report method in the daemon for track events ([328cff6](https://github.com/bluegardenproject/tracks/commit/328cff6a330de1f185a388ad72e1cb3c79f9e461))
* **v2:** a track's pull requests and their status ([f45017f](https://github.com/bluegardenproject/tracks/commit/f45017fea8f26c9ac803f24e10330e277986bb0a))
* **v2:** add a data source for screens, with fake details for demo tracks ([1e4acbc](https://github.com/bluegardenproject/tracks/commit/1e4acbc9f7894c99750d4d95dffc34f12394388d))
* **v2:** add a Track placeholder to Settings ([a719487](https://github.com/bluegardenproject/tracks/commit/a71948751c138a616a019be5fe089fad8a48325d))
* **v2:** add agents to the code map ([1ce5987](https://github.com/bluegardenproject/tracks/commit/1ce59874a20437c65635edd573a6a9612f3094c4))
* **v2:** add repo rules checked against git ([d42ca14](https://github.com/bluegardenproject/tracks/commit/d42ca14a0a7a612d260c7912ea72702f5150e711))
* **v2:** add table and accent border colour tokens ([55c8a9a](https://github.com/bluegardenproject/tracks/commit/55c8a9a001acfd8ab8a552073c50c0e2dc622480))
* **v2:** add the daemon and its protocol ([6eb7b94](https://github.com/bluegardenproject/tracks/commit/6eb7b943a063e889dfaee54d1856d27d02ce18dd))
* **v2:** add the New track form ([8c0f510](https://github.com/bluegardenproject/tracks/commit/8c0f5107965e7ea95d1826ed624c9811f7b9e13f))
* **v2:** add the settings file and the themes folder ([752c22f](https://github.com/bluegardenproject/tracks/commit/752c22fdfd5f8121a67f74bb771e86e507cfe66c))
* **v2:** add the SQLite store with forward migrations ([ac14f2a](https://github.com/bluegardenproject/tracks/commit/ac14f2a471d34c99d5f7952bd6c229d786d5be47))
* **v2:** add tracks to the domain and the store ([1ac1a05](https://github.com/bluegardenproject/tracks/commit/1ac1a05320b2b7923bcec9f982c668be4060e257))
* **v2:** agents stop and ask when a link can't be read ([7ebcbd8](https://github.com/bluegardenproject/tracks/commit/7ebcbd8a5a7c15a0f1fa606d64dad92a55d4c360))
* **v2:** archive and auto-archive ended tracks ([1573e2b](https://github.com/bluegardenproject/tracks/commit/1573e2bbf4503cb523a12116c8be9cbf0d688a4a))
* **v2:** archive button in the track details ([d396701](https://github.com/bluegardenproject/tracks/commit/d396701011bb3e4f04bc91206cb7a29333779899))
* **v2:** archive method, hourly auto-archive and a 1-minute PR poll ([162aa30](https://github.com/bluegardenproject/tracks/commit/162aa302788d383e4ef856032ba70a0284f657af))
* **v2:** Archive removes the branches too, and checks what would be lost ([a69ffb7](https://github.com/bluegardenproject/tracks/commit/a69ffb711e9a83a3a3fef26ac114047bd75fa78e))
* **v2:** archived tracks, left out of Station's list ([69e16e1](https://github.com/bluegardenproject/tracks/commit/69e16e132470774a9d63d41059cd7421eba13e6d))
* **v2:** ask to reopen the interrupted tracks at start ([ea1d32b](https://github.com/bluegardenproject/tracks/commit/ea1d32b9e3cb143cab9feae7717ccbe9145211b1))
* **v2:** auto-archive's keep leaves the branches too ([75963a5](https://github.com/bluegardenproject/tracks/commit/75963a5ee7a6f9b43e02585585bf7665596e92c7))
* **v2:** build agent command lines with v1's prompts ([a779f5d](https://github.com/bluegardenproject/tracks/commit/a779f5d8534ffdfae25290885bad9a7d27605274))
* **v2:** build the Engines tab ([a4318fc](https://github.com/bluegardenproject/tracks/commit/a4318fc5f37d7f92ad791c616250444c38317d22))
* **v2:** build the Repositories tab ([167bdc0](https://github.com/bluegardenproject/tracks/commit/167bdc0073debc2b5a11a36b28c0de318535c167))
* **v2:** build the Settings tab with theme files ([d12d9d9](https://github.com/bluegardenproject/tracks/commit/d12d9d9beec656034f1972b1a384911b7e626cf1))
* **v2:** check a track's worktrees before cleaning it ([ea09ce8](https://github.com/bluegardenproject/tracks/commit/ea09ce8404053f1ef8658a20ac05c4b1cb5ad78e))
* **v2:** chunk 4 is done, its plans go ([b92836b](https://github.com/bluegardenproject/tracks/commit/b92836bb8fd6ca04ff9c8c8bf7b2059513508060))
* **v2:** clean and re-create track worktrees ([b620211](https://github.com/bluegardenproject/tracks/commit/b620211e282cbb75ca62751e05d82a7f733a66a6))
* **v2:** clear action required when a Claude dialog closes without a hook ([81c0d98](https://github.com/bluegardenproject/tracks/commit/81c0d988f1052d5c3ce97ef9e34d0a167646e11c))
* **v2:** Close Tracks in Quick Access ([f215c6b](https://github.com/bluegardenproject/tracks/commit/f215c6be3fe1ddef76ec8db9abb17eb2de1838f9))
* **v2:** closed means archived, and an unarchived track is done ([da9d46a](https://github.com/bluegardenproject/tracks/commit/da9d46a91f9869a3aca4a0dcf9a89fefc20d7694))
* **v2:** closing Tracks and reopening are built ([fbe05bb](https://github.com/bluegardenproject/tracks/commit/fbe05bb09bc6a6ec4e9ba769e43f009691f5028b))
* **v2:** Create runs on the agent and model a request picks ([a46ea91](https://github.com/bluegardenproject/tracks/commit/a46ea911c93d70a6071cd6c4ba6e368348c5c817))
* **v2:** create tracks from the New track form ([cfbf6bf](https://github.com/bluegardenproject/tracks/commit/cfbf6bfaac79e5565511b41fda69c71ca338511f))
* **v2:** create, list and end tracks ([4d9244a](https://github.com/bluegardenproject/tracks/commit/4d9244a0ccecc165b4b485d9a46b73e171a2ee09))
* **v2:** Cursor's create-chat under XDG_CONFIG_HOME is a follow-up ([dad3316](https://github.com/bluegardenproject/tracks/commit/dad331672358f6b065d2c6acd0d8e5eda2f930a1))
* **v2:** Cursor's plan approval and mode switch mean action required ([8c16da8](https://github.com/bluegardenproject/tracks/commit/8c16da810e89e99bcb34e3977db33f8bd11e1442))
* **v2:** Cursor's questions are told by their count, not their title ([b74f333](https://github.com/bluegardenproject/tracks/commit/b74f333cf008b6d735043282118b02e762cbe895))
* **v2:** daemon sends notices ([a2e0e3d](https://github.com/bluegardenproject/tracks/commit/a2e0e3d2a0193ee1bb6a1a357f26db0dbfff76cf))
* **v2:** default agent and model per track type in settings ([ae09173](https://github.com/bluegardenproject/tracks/commit/ae091732a0ab099c6802c3f6059ced47990e0863))
* **v2:** derail a track through the daemon ([0ac1ae2](https://github.com/bluegardenproject/tracks/commit/0ac1ae2e02151a138e1fb8c5e8c74c17da9fadee))
* **v2:** Derail in Station ([cb69a5a](https://github.com/bluegardenproject/tracks/commit/cb69a5a32dc87669d667c76688ea47083b03e3dc))
* **v2:** Derail is built, chunk 4 is done ([e670eef](https://github.com/bluegardenproject/tracks/commit/e670eef07472d71f2af9db65c5291aa0f52f743e))
* **v2:** document creating tracks ([d97a4f3](https://github.com/bluegardenproject/tracks/commit/d97a4f3cb12037aaa75870c96dbc916b7a809a6f))
* **v2:** document Quick Access and the form in the plans ([4acec29](https://github.com/bluegardenproject/tracks/commit/4acec29a544217314a979a780f3e0981084b1064))
* **v2:** document resuming and cleaning tracks ([e8845e4](https://github.com/bluegardenproject/tracks/commit/e8845e46d68cae0b040d401adb47a00d796ca112))
* **v2:** document the form's Runs on field ([33c5649](https://github.com/bluegardenproject/tracks/commit/33c5649b2314e86c3d2f4952b0666d1821991959))
* **v2:** document the Repositories tab and storage ([ba4f02a](https://github.com/bluegardenproject/tracks/commit/ba4f02adcebbcc8950849c7d1f79911c8583138b))
* **v2:** document the Settings tab ([51b8af2](https://github.com/bluegardenproject/tracks/commit/51b8af29f38bc3c644a9249848b142d3da6f1026))
* **v2:** document the Station tab ([55eb6e2](https://github.com/bluegardenproject/tracks/commit/55eb6e257fc29ba64a243c16dfb1112fbce350e2))
* **v2:** document the Track section, overlay tokens and repo select list ([0a141f8](https://github.com/bluegardenproject/tracks/commit/0a141f8be62188d3fc53f23853909c33ad3e2d83))
* **v2:** draft and discard-draft calls ([eeb7df8](https://github.com/bluegardenproject/tracks/commit/eeb7df848025a2fd0cac7914a65077d1ab66ab2c))
* **v2:** draft status and store ([a9f17f2](https://github.com/bluegardenproject/tracks/commit/a9f17f22cef92beda4e600946d3eaa3e7b403a68))
* **v2:** drafts are built ([720eeec](https://github.com/bluegardenproject/tracks/commit/720eeecd29c76133e588caab2697479aed663b84))
* **v2:** drafts in Station ([bd7d816](https://github.com/bluegardenproject/tracks/commit/bd7d81631eaaef3f931ff3e422ca1741733e6e70))
* **v2:** drop Clean; Archive asks what would be lost ([ed93ea1](https://github.com/bluegardenproject/tracks/commit/ed93ea1b896d02ca8fa8cf88161ab455546074d1))
* **v2:** drop Make default until track types get their own ([c3d49b4](https://github.com/bluegardenproject/tracks/commit/c3d49b4b61c5fb7bfdc74a9faae78ec4deb626d2))
* **v2:** failed creations kept as drafts ([6a48e61](https://github.com/bluegardenproject/tracks/commit/6a48e61a0c556c274fb40d9c34bdee802be3f28c))
* **v2:** filter and unarchive methods, station list call ([2dcce34](https://github.com/bluegardenproject/tracks/commit/2dcce340cdbc442d15d723a003de3cc1ce5d44b0))
* **v2:** filter the picker as you type ([afde2a4](https://github.com/bluegardenproject/tracks/commit/afde2a4bf78b3de25cd96f7c01254884abf1cdd7))
* **v2:** filtered line, clear filter and unarchive in Station ([f450513](https://github.com/bluegardenproject/tracks/commit/f450513da0ea9704667946c621cc590588dfe087))
* **v2:** find the agent CLIs, their models and MCP servers ([c5c485f](https://github.com/bluegardenproject/tracks/commit/c5c485fed607a00ecc8e66e276931c215971fd2d))
* **v2:** footer refreshes every 15 seconds ([b2261d8](https://github.com/bluegardenproject/tracks/commit/b2261d88a97f91fcf9f59676f0529fd80d23b03e))
* **v2:** give overlays their own tokens ([20b9415](https://github.com/bluegardenproject/tracks/commit/20b941536c9291692659588e71f58f08d67f3e89))
* **v2:** install the Cursor rule once Cursor is added ([95e9446](https://github.com/bluegardenproject/tracks/commit/95e9446c575102745a8848967ea22a0efbdb9b49))
* **v2:** interrupted and reopen calls ([d78cfce](https://github.com/bluegardenproject/tracks/commit/d78cfce3bc0c48be281f89c03a5d982b7b4510ad))
* **v2:** keep a work track's renamed branch when cleaning it ([dc7ccb7](https://github.com/bluegardenproject/tracks/commit/dc7ccb7e3114dbd5af5501484d3fe1d250533c84))
* **v2:** keep the creator's scroll on the first click ([1fc90be](https://github.com/bluegardenproject/tracks/commit/1fc90be1089d0ab9fc458569763e990a1e4f7846))
* **v2:** keep the engines in settings.yaml ([a904c62](https://github.com/bluegardenproject/tracks/commit/a904c62c6668122cd125ae2e67f913d1cf69c703))
* **v2:** keep v1's agent prompts ([380d430](https://github.com/bluegardenproject/tracks/commit/380d430c0e98f66e4084b2f4c76bcf509ecb65ce))
* **v2:** list the tmux prefix keys in one place ([171f519](https://github.com/bluegardenproject/tracks/commit/171f519ff6009db5b18d985e26952a1c09a40d70))
* **v2:** list track windows with their kind, repo and directory ([817e607](https://github.com/bluegardenproject/tracks/commit/817e607c15f41aa8248cfb98e8cf46215bdf8748))
* **v2:** list tracks from the daemon and pick the default engine ([25a8061](https://github.com/bluegardenproject/tracks/commit/25a806181e53fc953d83f527c8886aa6be4bd9f4))
* **v2:** make a track's worktrees ([8483164](https://github.com/bluegardenproject/tracks/commit/84831640dea08b7d3976e1d5a59f634e58a784f5))
* **v2:** map the agents' hooks to track events, and write their per-track files ([5b2d1d3](https://github.com/bluegardenproject/tracks/commit/5b2d1d396aea1eedc23c4f4be240f0d2bdd3462a))
* **v2:** mark a respawned window only once its agent restarted ([4b23334](https://github.com/bluegardenproject/tracks/commit/4b233343e91bd0143d6eb8177001327d6aa28a5e))
* **v2:** mark a track that needs you in the footer ([17ff69b](https://github.com/bluegardenproject/tracks/commit/17ff69b3875fa9bfaa1fd89d951c3497d0dd5153))
* **v2:** New track form starts a draft again ([11c6cc3](https://github.com/bluegardenproject/tracks/commit/11c6cc35c8f7d21dbf60588634791da94b994d13))
* **v2:** new tracks run on their type's agent and model ([dbcb91b](https://github.com/bluegardenproject/tracks/commit/dbcb91b638b73c57622f06fa4003d0a185212933))
* **v2:** no Proxy tab until the redesign ([fe1197a](https://github.com/bluegardenproject/tracks/commit/fe1197a66c7697f1653a239c60d2c959c6d1518c))
* **v2:** no Resume after Clean, and ask before re-creating a missing worktree ([33062b6](https://github.com/bluegardenproject/tracks/commit/33062b6be1b318198d9cb37012b98488f86b1aed))
* **v2:** note how the pane check tells a dialog ([387a243](https://github.com/bluegardenproject/tracks/commit/387a243dccce5fd10e30614724d4dc8422eb17bd))
* **v2:** notices for status and PR changes ([37e6c64](https://github.com/bluegardenproject/tracks/commit/37e6c6465d07b8fd150036e821954121d2f4b612))
* **v2:** notices that report something done go after 5 seconds, with a ✕ ([70fe867](https://github.com/bluegardenproject/tracks/commit/70fe867973d051106964f34874f80f3fd43be031))
* **v2:** notification settings ([2bb0740](https://github.com/bluegardenproject/tracks/commit/2bb0740c99a96c576cc0b4d43b73040390cbb499))
* **v2:** notification toggles in Settings → General ([4e3cac2](https://github.com/bluegardenproject/tracks/commit/4e3cac2d8797573f4f7106f2ba656b718f34b26c))
* **v2:** notifications are built ([62486d8](https://github.com/bluegardenproject/tracks/commit/62486d82f71e4b373a297a4767c22b11badb2b16))
* **v2:** notifier for macOS and the bell ([c4d3ca8](https://github.com/bluegardenproject/tracks/commit/c4d3ca8c18def37297fac12ce902f068d5ecc7c2))
* **v2:** offer Add new Track on an empty Station ([d1fee13](https://github.com/bluegardenproject/tracks/commit/d1fee135911e3fff5ecb02b6e9ce5af945303570))
* **v2:** open popups on a tmux client ([a145f99](https://github.com/bluegardenproject/tracks/commit/a145f996c431bb2e63bcbe904d4c1dfecd7ace39))
* **v2:** open Quick Access on Ctrl+b q ([b1bc63b](https://github.com/bluegardenproject/tracks/commit/b1bc63bda3017761543a40ae665ea6bdfbd8e59d))
* **v2:** pick the agent and model in the New track form ([5cc18c0](https://github.com/bluegardenproject/tracks/commit/5cc18c0eb56e6cb59300a985b3e104e5fcbab040))
* **v2:** pick the form's repos from a select list ([857a69b](https://github.com/bluegardenproject/tracks/commit/857a69b06eea61454333d1e23e3c39cefa0695d1))
* **v2:** plan 09 is built ([c76e5dd](https://github.com/bluegardenproject/tracks/commit/c76e5ddbeb30e8f6c173fb00ca68e2400fb03700))
* **v2:** plan 10 is built ([3c547d7](https://github.com/bluegardenproject/tracks/commit/3c547d7f18e7211d7abb75d36af9885cb8bd3ef0))
* **v2:** plan 11 is built ([fa64c80](https://github.com/bluegardenproject/tracks/commit/fa64c80ed6ff6993e906985e35b2cdd25bfb0b94))
* **v2:** plan 13, Archive replaces Clean ([70e17ee](https://github.com/bluegardenproject/tracks/commit/70e17eeaa29a52ad441af03054362a8c8f344c3e))
* **v2:** plan 14 is built ([0d27d3a](https://github.com/bluegardenproject/tracks/commit/0d27d3a02857fe6d9291faf6a4b535843984e3be))
* **v2:** plan 15 is built ([c93553a](https://github.com/bluegardenproject/tracks/commit/c93553a0f6401e725fd2eea5dc6e57f15cae1665))
* **v2:** plan archive and filters ([d4efd25](https://github.com/bluegardenproject/tracks/commit/d4efd255992b67e7608f0e57359ce8be46b8b59d))
* **v2:** plan change stream and derail ([dafb00d](https://github.com/bluegardenproject/tracks/commit/dafb00df61c5e0475d2b0a203492a1e093f4422b))
* **v2:** plan closing Tracks and reopening its tracks ([5823bcf](https://github.com/bluegardenproject/tracks/commit/5823bcf35c16454805ff3c0e252ee26d38bf04e6))
* **v2:** plan creating tracks ([a341b9f](https://github.com/bluegardenproject/tracks/commit/a341b9fce91789e40567a70325ef44ca4e1d152c))
* **v2:** plan default agents and models per track type ([9f0ac21](https://github.com/bluegardenproject/tracks/commit/9f0ac213f8770e22e3772e85ab7ae81eb7dac221))
* **v2:** plan drafts ([47b0c49](https://github.com/bluegardenproject/tracks/commit/47b0c49760dc0ab2b45a1d6655b042164158844f))
* **v2:** plan notifications ([57327c8](https://github.com/bluegardenproject/tracks/commit/57327c8a025ffe74c519e0745dc1fb7508e83ef5))
* **v2:** plan Quick Access and the New track form ([3fa8c5c](https://github.com/bluegardenproject/tracks/commit/3fa8c5c3ea96d3c58ee2c2f4424215abe69b87d3))
* **v2:** plan resuming and cleaning tracks ([2d331a8](https://github.com/bluegardenproject/tracks/commit/2d331a89d3ab1f790a96681416cccfb30e93f4d2))
* **v2:** plan supervision ([3e52fae](https://github.com/bluegardenproject/tracks/commit/3e52fae3b6ee457b44dc0a67ed278e62fc788dff))
* **v2:** plan the Engines tab ([1deae60](https://github.com/bluegardenproject/tracks/commit/1deae60b59ba369f6429416f8c59cb9d443946e7))
* **v2:** plan the Homebrew install ([566deb2](https://github.com/bluegardenproject/tracks/commit/566deb22173aef976749afb16f6dbf36719b2959))
* **v2:** plan the tracks commands ([d2ced26](https://github.com/bluegardenproject/tracks/commit/d2ced26196e1b2562fa633427a09ff392a840563))
* **v2:** plan track status ([6acb003](https://github.com/bluegardenproject/tracks/commit/6acb00346de17bd4a5d98cafbd4a61c64d29620a))
* **v2:** poll GitHub for the tracks' pull requests every 2 minutes ([866669f](https://github.com/bluegardenproject/tracks/commit/866669f16bee82d55afdaf94a07e4bf6fbb42da6))
* **v2:** promote an Ask or Plan track to a Work track ([e07c6aa](https://github.com/bluegardenproject/tracks/commit/e07c6aaf5e03b24f27f6eb5092a346fe52cfd0ee))
* **v2:** promote from Station ([1a8e87a](https://github.com/bluegardenproject/tracks/commit/1a8e87aea7b3434a2bbef7bf3b3c3cffc8980851))
* **v2:** prompts leave out dev servers until the proxy redesign ([94953fe](https://github.com/bluegardenproject/tracks/commit/94953fe97b933b58a904648d0aa8cdea2ba08e34))
* **v2:** questions asked in plain text are a follow-up ([44b5962](https://github.com/bluegardenproject/tracks/commit/44b59629d9e244b2168a925a1297647f0b3c3174))
* **v2:** record ended tracks' cleaning and reopening ([f416ac7](https://github.com/bluegardenproject/tracks/commit/f416ac7e2b63f662b5736cf28addb774e3977ca0))
* **v2:** record the agents that exit ([e1a88b8](https://github.com/bluegardenproject/tracks/commit/e1a88b84e992f1ec71bf011cdf8ccdbd1fef7c8c))
* **v2:** record the pull requests the agents' hooks report ([3bd4b66](https://github.com/bluegardenproject/tracks/commit/3bd4b66987537ebb3d034326ceff9127a262a51b))
* **v2:** reflow the status comment ([62c4b13](https://github.com/bluegardenproject/tracks/commit/62c4b1330016534cff89d3aad509998dd323ef16))
* **v2:** remove the playground ([5db65b2](https://github.com/bluegardenproject/tracks/commit/5db65b2e026432eb5e828ae07a6497e798d3e670))
* **v2:** respawn a track window's agent in place ([ac149bc](https://github.com/bluegardenproject/tracks/commit/ac149bc29601eea91e053d7242c8e8887cc1f041))
* **v2:** restart a track's agent in its pane ([6fcf6e0](https://github.com/bluegardenproject/tracks/commit/6fcf6e0e1a4afb5f3f616d05ebb39b38a0e63c8f))
* **v2:** restart from Station ([32fb2c9](https://github.com/bluegardenproject/tracks/commit/32fb2c9f99bd5b06424880418f98028a50d8d35c))
* **v2:** resume and clean ended tracks ([84f32d0](https://github.com/bluegardenproject/tracks/commit/84f32d0639db5fdcdef428b7c3745e6e168f9745))
* **v2:** resume and clean ended tracks in Station ([188577a](https://github.com/bluegardenproject/tracks/commit/188577ab26d9cd2be00677abd01cfc6723c7ef31))
* **v2:** resume and clean methods in the daemon ([908d017](https://github.com/bluegardenproject/tracks/commit/908d0170754247a94466834d20578f5b746fda01))
* **v2:** resume command lines for Claude and Cursor ([f09bab6](https://github.com/bluegardenproject/tracks/commit/f09bab611388a8013c4720c7fc09d09d8f899849))
* **v2:** Resume re-creates a deleted branch from origin or the base ([fd8b277](https://github.com/bluegardenproject/tracks/commit/fd8b27726d15a49cac9e5fc454dd0bda1500ed89))
* **v2:** save a promoted track, and read a Claude session's last reply ([4613202](https://github.com/bluegardenproject/tracks/commit/4613202ae2f5a00a57a26d8d7f8bcf3fb2fb2130))
* **v2:** save the agent's exit code in its window ([60d862b](https://github.com/bluegardenproject/tracks/commit/60d862bcdd2d76cc8ad548728b1c184ce301d688))
* **v2:** Select engine sits below Name ([b6e885a](https://github.com/bluegardenproject/tracks/commit/b6e885acce7f3f618077d53ac770535560ca1503))
* **v2:** Settings → Tracks sets each track type's agent and model ([a7233aa](https://github.com/bluegardenproject/tracks/commit/a7233aac754fb749d26bba2657cc7fe35edb7338))
* **v2:** Settings frames start with a blank line, engine frames don't light up ([042b639](https://github.com/bluegardenproject/tracks/commit/042b6395e3d4f69bd37fa236b3d5d5a6b8db4038))
* **v2:** share the table and text input between screens ([93ea96b](https://github.com/bluegardenproject/tracks/commit/93ea96bb43f4f93070c66e8a9694ec3358eb99e1))
* **v2:** show each track's status in Station, in its colour ([571926a](https://github.com/bluegardenproject/tracks/commit/571926aeb457c4e0f3f0686ef7595710232428a8))
* **v2:** show statuses as badges ([4fd54e8](https://github.com/bluegardenproject/tracks/commit/4fd54e87aad6c9bbd1bcc1b32ba395a5bf4e992e))
* **v2:** show the PR status in Station and the details ([83df881](https://github.com/bluegardenproject/tracks/commit/83df881d0d8816f05c1b182d5131142717058d20))
* **v2:** show the track list, details and Fast Track placeholder on Station ([82acf19](https://github.com/bluegardenproject/tracks/commit/82acf1946a6dfa7694494a57823aa5d8387400c3))
* **v2:** show when a track was created in its details ([1b7ff2a](https://github.com/bluegardenproject/tracks/commit/1b7ff2acc6d624a2b87cfe074923184de725972b))
* **v2:** sort theme files by file name and fix creator hover ([01cdee7](https://github.com/bluegardenproject/tracks/commit/01cdee7beae24f89d95b6158caa33c7a53534298))
* **v2:** start each track's agent with its hooks ([3ed8bda](https://github.com/bluegardenproject/tracks/commit/3ed8bda75af94bd0d59804c31392eb0fca7a3284))
* **v2:** station filter stored, and the filtered tracks query ([0413bd5](https://github.com/bluegardenproject/tracks/commit/0413bd5549254ba9059cf0218664369963f045e3))
* **v2:** Station follows the change stream ([cb4371f](https://github.com/bluegardenproject/tracks/commit/cb4371f67d5aa330a58da799b008a604ca097dba))
* **v2:** station list under the stored filter ([014de15](https://github.com/bluegardenproject/tracks/commit/014de154cebae162c95b88584ace4088a868966d))
* **v2:** Station lists tracks in the order they were created ([2b45f56](https://github.com/bluegardenproject/tracks/commit/2b45f56d411d6bf213ea1538071ffa016780f0dd))
* **v2:** Station shows the name typed, and the slug in the details ([e9e0b49](https://github.com/bluegardenproject/tracks/commit/e9e0b49d211fe9dd5cd9efdbf3edc0efbb27bfa7))
* **v2:** Station's cost column, read from Claude's transcripts ([7ba913d](https://github.com/bluegardenproject/tracks/commit/7ba913d8b74a823bf9d5de21d5497e2907e83540))
* **v2:** Station's model column, the model picked at creation ([824188a](https://github.com/bluegardenproject/tracks/commit/824188aeefafdc68424aab0b2fe6c101e5731d60))
* **v2:** store the tracks' pull requests ([f242c06](https://github.com/bluegardenproject/tracks/commit/f242c062e437e51b41f5f198fc41c8876b5250eb))
* **v2:** store: delete a track ([852bf68](https://github.com/bluegardenproject/tracks/commit/852bf689540f63524b4d6f3dab2c9286477fe1db))
* **v2:** stream track changes from the daemon ([123e90f](https://github.com/bluegardenproject/tracks/commit/123e90fec8e6a0d3cce4c27203a73e838ee63f97))
* **v2:** tell Cursor's dialogs from its pane ([cf89b59](https://github.com/bluegardenproject/tracks/commit/cf89b59c956a41c8c9f1a77fa5f5b8df22349168))
* **v2:** tell when tracks change ([7839f80](https://github.com/bluegardenproject/tracks/commit/7839f80909a1057eaf3e3aea1315671c4d3a7c17))
* **v2:** the change stream is built ([9bb13e5](https://github.com/bluegardenproject/tracks/commit/9bb13e5f152557709f33cc16aab4aae746b17674))
* **v2:** the daemon exits when the tmux server restarts ([0ef892f](https://github.com/bluegardenproject/tracks/commit/0ef892f13d05b6c53dcd584efef26318f5b9696c))
* **v2:** the daemon interrupts the tracks left without a window ([6653441](https://github.com/bluegardenproject/tracks/commit/665344133c7f5367dbd860f1c331cab796855ffa))
* **v2:** the default theme's overlay and focus colours ([1ae37a3](https://github.com/bluegardenproject/tracks/commit/1ae37a3e0de41be9706e639ba22a0147c3b7cee8))
* **v2:** the details drop the Open PR button ([ec7f001](https://github.com/bluegardenproject/tracks/commit/ec7f0011b29f784cfb92340186e9cf65fed01bed))
* **v2:** the error and agent exited statuses ([dd8f927](https://github.com/bluegardenproject/tracks/commit/dd8f927d741a09adece24149a64fdf4f24f69527))
* **v2:** the filter popup leaves out closed, which Archived only picks ([82db5ad](https://github.com/bluegardenproject/tracks/commit/82db5adcc330159f90f19aca03fdef15fe4641d7))
* **v2:** the folder trust prompt is a follow-up ([ac5a65f](https://github.com/bluegardenproject/tracks/commit/ac5a65fb66a38e669b86e5c3f804242bb1ad417f))
* **v2:** the New track form says what each type runs on ([9933bb1](https://github.com/bluegardenproject/tracks/commit/9933bb13c79601d77e81e9f71f21e9eb2fd0aa9e))
* **v2:** the v2.0.0 scope, proxy after the release ([632e5f3](https://github.com/bluegardenproject/tracks/commit/632e5f391b3453210af0e56305477c1c77ec6fa0))
* **v2:** the version shows only in About ([98f7b1e](https://github.com/bluegardenproject/tracks/commit/98f7b1e33a4bc5c8c1b797a8385cf136dd1c1549))
* **v2:** track filter and its matching ([1659d5b](https://github.com/bluegardenproject/tracks/commit/1659d5be4b98b4df8c2ac519ae09b8e5f1e0990f))
* **v2:** track status from one state, changed only by reported events ([02c8ec4](https://github.com/bluegardenproject/tracks/commit/02c8ec4cf8098fde8eabd549e7909fe33faa7faf))
* **v2:** tracks add-repo and the add-repo skill ([a27d3fb](https://github.com/bluegardenproject/tracks/commit/a27d3fbc054ae0d6088f1395052d754a75721d78))
* **v2:** tracks filter in Quick Access ([ee5cc88](https://github.com/bluegardenproject/tracks/commit/ee5cc88c6e88d2d0a3d9a3e78e101789e301def7))
* **v2:** tracks filter popup ([fc5c408](https://github.com/bluegardenproject/tracks/commit/fc5c408d5ad6b9891a938bb8324c4c490318f174))
* **v2:** tracks history group in Settings → Tracks ([96980f9](https://github.com/bluegardenproject/tracks/commit/96980f9d9a9da611c5c83e7b113dd1a766b8a5b6))
* **v2:** tracks history settings ([2a4ffec](https://github.com/bluegardenproject/tracks/commit/2a4ffeccbfc2f7395639ee98f89d5ef673a2dff1))
* **v2:** tracks interrupted when Tracks closes ([1c3b916](https://github.com/bluegardenproject/tracks/commit/1c3b91659ea96436b75c1e3216b82dcbdfc926bd))
* **v2:** tracks promote ([154f47d](https://github.com/bluegardenproject/tracks/commit/154f47dc6c4689d5ab73bba9d6e3e25d42e7468d))
* **v2:** tracks restart ([97632fc](https://github.com/bluegardenproject/tracks/commit/97632fc37fdfa37840d46413604971a7b58115ab))
* **v2:** tracks review runs the reviewer as a second Cursor agent ([6b55e12](https://github.com/bluegardenproject/tracks/commit/6b55e1226e32862b5c1398a8a34b7e041470476c))
* **v2:** tracks terminal opens a terminal in the track's window ([d88f597](https://github.com/bluegardenproject/tracks/commit/d88f597b6afa1853056483d476fb24694d76e6f9))
* **v2:** workspace: what derailing loses, and derailing ([2fb3071](https://github.com/bluegardenproject/tracks/commit/2fb3071f3d0d83d53b974d98a8251612f924959c))

## [1.3.0](https://github.com/bluegardenproject/tracks/compare/v1.2.0...v1.3.0) (2026-09-25)


### Features

* **tui:** share one palette and form theme across screens ([38909e0](https://github.com/bluegardenproject/tracks/commit/38909e0b8fd6d776de9353f5f6248fcf20d902c1))


### Bug Fixes

* **bootstrap:** don't restart a daemon started from another binary ([a9ebf01](https://github.com/bluegardenproject/tracks/commit/a9ebf016592cf98ec0431f280b0a6fb48abcc451))


### Documentation

* add Tracks v2 masterplan and first feature plans ([1dd0515](https://github.com/bluegardenproject/tracks/commit/1dd0515a1fff03c5a723527f7fa86d4879ae3f7f))


### Miscellaneous

* **deps:** bump bubbles to v1.0.0 and bubbletea to v1.3.10 ([2d16cbc](https://github.com/bluegardenproject/tracks/commit/2d16cbc48285e2a13591783a6dd7b2b88c831a61))
* **v2:** add AGENTS.md and v2 paths ([850dfea](https://github.com/bluegardenproject/tracks/commit/850dfeacdfef6825a5da409e8d3b19b2de5cf30f))
* **v2:** add design tokens and a first Tracks window ([0f6b39b](https://github.com/bluegardenproject/tracks/commit/0f6b39b9250183ad4701b7f6af40ebaf49ae8e34))
* **v2:** demo playground with fake tracks ([5af802f](https://github.com/bluegardenproject/tracks/commit/5af802f43935e2587e6d5a646401078cffff321c))
* **v2:** four-row footer with track navigation and system info ([de890cc](https://github.com/bluegardenproject/tracks/commit/de890cc6bf2ec9fc9177487dc7a9b674d3067805))
* **v2:** run v2 on its own tmux server ([61c3a38](https://github.com/bluegardenproject/tracks/commit/61c3a3844b7724a410a37625522860de3071a60c))
* **v2:** theme creator in the Tracks window ([ca8cb90](https://github.com/bluegardenproject/tracks/commit/ca8cb906e42e706dd2d3b84b529192201118c763))
* **v2:** Tracks keys behind the tmux prefix ([a812788](https://github.com/bluegardenproject/tracks/commit/a812788e50da2dfbaf419a7e41ec2f18b660a035))
* **v2:** trackwin builds track windows ([c199619](https://github.com/bluegardenproject/tracks/commit/c199619ab568ae98d778832991d0982a8c9675a9))
* **v2:** update plans for the footer and theme creator ([91ef18c](https://github.com/bluegardenproject/tracks/commit/91ef18c659079e01344d5ab7fbb220b3947510cf))

## [1.2.0](https://github.com/bluegardenproject/tracks/compare/v1.1.1...v1.2.0) (2026-09-22)


### Features

* add worktree terminal panes ([12eae54](https://github.com/bluegardenproject/tracks/commit/12eae5419f4f3bba3ac6b7cf4113f584fcef1e08))


### Bug Fixes

* **install:** refuse a download SHA256SUMS can't vouch for ([d651273](https://github.com/bluegardenproject/tracks/commit/d65127344cfeaa2d9d0a96a3563013defbd51ac0))
* use portable tmux split sizing ([4bc8577](https://github.com/bluegardenproject/tracks/commit/4bc857740609b7182e2dfba8f2a4e0d4d29c454d))

## [1.1.1](https://github.com/bluegardenproject/tracks/compare/v1.1.0...v1.1.1) (2026-09-07)


### Bug Fixes

* **daemon:** only adopt GitHub URLs from the PR marker ([37bc7f1](https://github.com/bluegardenproject/tracks/commit/37bc7f12f01312013ca0494acef12d35bc574f69))
* **dashboard:** strip control characters from externally-supplied text ([d2e5b20](https://github.com/bluegardenproject/tracks/commit/d2e5b201dcbd211945f717fb633d7f25997089b9))
* **notify:** escape backslashes before quotes in AppleScript strings ([36d5980](https://github.com/bluegardenproject/tracks/commit/36d59801d63dbf330a270bf3e2e7db6408bf01fe))
* **update:** finish the de-branding pass and the verify story ([edd40b9](https://github.com/bluegardenproject/tracks/commit/edd40b92bf3fe31115e295526a9b918991588208))
* **update:** verify a release download before running it ([77668c1](https://github.com/bluegardenproject/tracks/commit/77668c1ba115bd21c6198e7a2f4aad3135b1018a))


### Miscellaneous

* drop project-specific references from docs and fixtures ([53b4e13](https://github.com/bluegardenproject/tracks/commit/53b4e1353a5933ed1b6069006f0029191cb100e5))

## [1.1.0](https://github.com/bluegardenproject/tracks/compare/v1.0.1...v1.1.0) (2026-09-02)


### Features

* **cursor:** don't cost a Cursor track against Anthropic rates ([3a69492](https://github.com/bluegardenproject/tracks/commit/3a6949228722c7c58395f67128159a1f3325cfb9))
* **cursor:** install the tracks rule into ~/.cursor/rules ([ef1a6ce](https://github.com/bluegardenproject/tracks/commit/ef1a6cee9a9f41d1942b4541309982c30e0b601e))
* **cursor:** launch Cursor Agent as a track's provider ([ed272f6](https://github.com/bluegardenproject/tracks/commit/ed272f60f89efd0d3551a8faee91367b673b24fe))
* **cursor:** review with a second agent, not with itself ([f1f1bea](https://github.com/bluegardenproject/tracks/commit/f1f1bead04ea5887a8dc4f078ba9726206628572))
* **tracks:** carry a provider on every track ([b2ccf2c](https://github.com/bluegardenproject/tracks/commit/b2ccf2ca85b431f8e4eda56fbca3cc26e558cb70))
* **tracks:** launch the provider the track was created with ([f3a6ecf](https://github.com/bluegardenproject/tracks/commit/f3a6ecfe66fdbe87ba3ea67c51028b73f2ca0327))
* **tracks:** pick a provider when creating a track ([b2e3955](https://github.com/bluegardenproject/tracks/commit/b2e395562292e756f6c92f57499220fb1bdee750))
* **tracks:** reopen every track you had open, not just the running ones ([4940eaa](https://github.com/bluegardenproject/tracks/commit/4940eaae77b47281ed7bf495de1fc27c289e7e6f))


### Bug Fixes

* **cursor:** rune-safe truncation, and drop a constant nothing reads ([936b1a3](https://github.com/bluegardenproject/tracks/commit/936b1a369638263da343d8844760e40ef01c92a9))
* **daemon:** never overwrite a home-directory file tracks doesn't own ([9ab3d20](https://github.com/bluegardenproject/tracks/commit/9ab3d209c6572e9532cea102bfa888e7496eda33))
* **tracks:** diff the changed-file list from the merge-base ([fbbf16f](https://github.com/bluegardenproject/tracks/commit/fbbf16fbad9e3922713709e8922722e480084a67))
* **tracks:** keep a PR-open track reachable across a restart ([8221e4e](https://github.com/bluegardenproject/tracks/commit/8221e4e3cb7d077be2fe08572d9672fb404b0389))
* **tracks:** say when the detail panel couldn't read a repo ([62c06bf](https://github.com/bluegardenproject/tracks/commit/62c06bf5025ef7b4da6b1b6b15fb62a5d543227b))


### Performance Improvements

* **tracks:** rate-limit the git polling behind the dashboard ([94259fd](https://github.com/bluegardenproject/tracks/commit/94259fdb6c0f44a09f82a7f3d055aa210d28bbbd))


### Code Refactoring

* **agent:** share the pane scaffolding between providers ([6a3bdcd](https://github.com/bluegardenproject/tracks/commit/6a3bdcdd2ea11a6ae4e2e129715e2417a2ccd3eb))
* **shellx:** one shell-quoting implementation, two named behaviours ([d9867cb](https://github.com/bluegardenproject/tracks/commit/d9867cbd660ad3a666f36124089da76c1a0f025b))
* **shellx:** quote globs, and stop the test passing by accident ([73ed28f](https://github.com/bluegardenproject/tracks/commit/73ed28fce09fb6fe4c9bdf2942a31056e134af3b))
* **tracks:** clean up the diffstat leftovers ([621e196](https://github.com/bluegardenproject/tracks/commit/621e196ef106f8b9affa315c78f9d62c3d2c73ee))
* **tracks:** drop the per-tick diffstat ([d770f13](https://github.com/bluegardenproject/tracks/commit/d770f138e807973cd837aa4770a429e074bfe056))


### Documentation

* **roadmap:** the shellQuote half of "shared helpers" is done ([5f54d1e](https://github.com/bluegardenproject/tracks/commit/5f54d1e672b604085cc2e7fe7af27b0cb799fb9b))

## [1.0.1](https://github.com/bluegardenproject/tracks/compare/v1.0.0...v1.0.1) (2026-08-27)


### Documentation

* **design:** Cursor CLI integration research and design ([89a1e9d](https://github.com/bluegardenproject/tracks/commit/89a1e9de5eb2d7771d2dff977c073c500d54af80))

## [1.0.0](https://github.com/bluegardenproject/tracks/compare/v0.7.0...v1.0.0) (2026-08-27)


### ⚠ BREAKING CHANGES

* **state:** state.json is written as schema_version 5. An older tracks binary refuses to load it ("schema_version 5 newer than supported") and will not start against a state file this version has written. Downgrading means restoring a backup of state.json.

### Features

* **dashboard:** drop the CHANGES column from the track table ([a90c21c](https://github.com/bluegardenproject/tracks/commit/a90c21c2583ca084ce6f8b05077da08a5665265c))
* **dashboard:** report a track's sub-agent model alongside its own ([d25e43c](https://github.com/bluegardenproject/tracks/commit/d25e43c1b1278b2a35fec08d63b0eaf25ee979ea))
* **dashboard:** show each track's model, and size the table to the terminal ([2919eb2](https://github.com/bluegardenproject/tracks/commit/2919eb296508df15634c893fe4b51f0f3000e9a3))
* **settings:** add a Models section ([8ec3ba0](https://github.com/bluegardenproject/tracks/commit/8ec3ba0c926f96cc7d89d542b8a174114190ccd9))
* **tracks:** choose a model when creating a track ([522ee3c](https://github.com/bluegardenproject/tracks/commit/522ee3c68ab330e04c745621e9b20fbea0397646))
* **tracks:** name tmux tabs after the track, not the track id ([115f6a1](https://github.com/bluegardenproject/tracks/commit/115f6a1f989ab9165a3625c5fe9e8d832ebf1252))


### Bug Fixes

* **usage:** price each model version, not each family ([c4207ad](https://github.com/bluegardenproject/tracks/commit/c4207ad3d9014c4df3716ad927837e63c563fe2f))


### Code Refactoring

* **state:** move review/doc fields into sub-structs, drop LogPath ([f577705](https://github.com/bluegardenproject/tracks/commit/f577705196b8b93f27f942fdc2d864e4fdbb602e))
* **state:** name the derived model ObservedModel ([270d079](https://github.com/bluegardenproject/tracks/commit/270d07900dd9521fab062dd066ffe7820ad35938))


### Documentation

* **roadmap:** record the open 1.0 review backlog ([bc9a2af](https://github.com/bluegardenproject/tracks/commit/bc9a2af2e448c8f25d80c244e733e00dca2850d2))
* **roadmap:** record the proxy review findings ([6fa324d](https://github.com/bluegardenproject/tracks/commit/6fa324dedffe7bf680b5d1b4a7dd15abe99738ba))

## [0.7.0](https://github.com/bluegardenproject/tracks/compare/v0.6.0...v0.7.0) (2026-08-18)


### Features

* **proxy:** user-defined stable ports, decoupled from service config ([c5a8fdb](https://github.com/bluegardenproject/tracks/commit/c5a8fdb5d848622cf70a5296d24384eeb9cc1d55))
* **review:** sharpen the doc reviewer's opinions and findings ([f27d0d8](https://github.com/bluegardenproject/tracks/commit/f27d0d87cb046b774d88b080b0ea9edcac3428b9))

## [0.6.0](https://github.com/bluegardenproject/tracks/compare/v0.5.0...v0.6.0) (2026-08-14)


### Features

* **daemon:** write the daemon log to a file ([5be2459](https://github.com/bluegardenproject/tracks/commit/5be2459b097d2e84e573d08aed10a44c98b0b224))
* **menu:** check for updates and self-install a newer release ([a2c246d](https://github.com/bluegardenproject/tracks/commit/a2c246d495d710c3cc67877aa2bcee3e28930fa0))
* **services:** make the ready probe and post_start hooks real ([64fe38b](https://github.com/bluegardenproject/tracks/commit/64fe38b83db9f41727e2f6091e0778086fad91ad))


### Bug Fixes

* **daemon:** guard the supervisor's observation state with its own lock ([5032f81](https://github.com/bluegardenproject/tracks/commit/5032f815a6483fb1fb820e0bcdf4a5e5daaaf963))
* **daemon:** log the state writes that used to fail silently ([21602c2](https://github.com/bluegardenproject/tracks/commit/21602c2e092b6f57e97e7b305d179f2b07a863e0))
* **proxy:** bind stable ports to loopback, and follow config reloads ([a842810](https://github.com/bluegardenproject/tracks/commit/a842810974bd99125a86ca84c278acc7839f727f))
* remove the permission-prompt path that nothing fed ([160c5f0](https://github.com/bluegardenproject/tracks/commit/160c5f06c89ff21b789c57eab4226fd97b08f815))
* **update:** count only the tracks the restart actually interrupts ([faf9081](https://github.com/bluegardenproject/tracks/commit/faf9081364c2d5d2b9cdb3cf534382fd1683c514))
* **update:** state the real cost of the next tracks run ([e521a50](https://github.com/bluegardenproject/tracks/commit/e521a50eff0e55f4bae3a447cc9b0a174b8ae0de))


### Code Refactoring

* drop code left dead by earlier rewrites ([bf25d2f](https://github.com/bluegardenproject/tracks/commit/bf25d2fad6f3bfaf49fced21a6a518c05262a974))


### Miscellaneous

* add MIT LICENSE ([cace206](https://github.com/bluegardenproject/tracks/commit/cace206076344fedec9a1d79e4f97d1ded15a4ff))

## [0.5.0](https://github.com/bluegardenproject/tracks/compare/v0.4.1...v0.5.0) (2026-08-07)


### Features

* **review:** candor dial, optional claim check, opinion section ([0d29f66](https://github.com/bluegardenproject/tracks/commit/0d29f6650a84838114bd40d5500c333dbd0ae0f2))
* **state:** per-PR track state with pr open / pr merged statuses ([7f3f332](https://github.com/bluegardenproject/tracks/commit/7f3f332df6a3410e7b5b3b75406224d9d8d13f11))


### Bug Fixes

* **services:** stop the runner tests racing the shell and the kernel ([4156bfe](https://github.com/bluegardenproject/tracks/commit/4156bfe85e6b18194c853420e087b79489918290))

## [0.4.1](https://github.com/bluegardenproject/tracks/compare/v0.4.0...v0.4.1) (2026-08-06)


### Bug Fixes

* **release:** drop the windows build that blocked binary uploads ([69d857b](https://github.com/bluegardenproject/tracks/commit/69d857b41719d6e408ec16c5bdf186f083fbc843))

## [0.4.0](https://github.com/bluegardenproject/tracks/compare/v0.3.1...v0.4.0) (2026-08-05)


### Features

* **claude:** skip pre-push review for docs-only diffs ([adc019d](https://github.com/bluegardenproject/tracks/commit/adc019dae9a60c259e86b94dd840bb1cce9dcaf8))
* **cmd:** reopen the tracks interrupted by the last shutdown ([db6936c](https://github.com/bluegardenproject/tracks/commit/db6936cc6c213eeed4b7baec54f0dffb9851967c))
* **daemon:** record interrupted tracks instead of erroring them ([3a711d6](https://github.com/bluegardenproject/tracks/commit/3a711d68518957596b000c74488c96462cf56c7b))
* **newtrack:** add doc-review track type ([866df94](https://github.com/bluegardenproject/tracks/commit/866df94bbc4f087c37509830113bbf6e310ae194))


### Documentation

* **roadmap:** log the test-spawns-into-live-tmux bug as high prio ([1d0bb67](https://github.com/bluegardenproject/tracks/commit/1d0bb671fe35b957dc8244c6d7d9a853badac103))

## [0.3.1](https://github.com/bluegardenproject/tracks/compare/v0.3.0...v0.3.1) (2026-07-21)


### Miscellaneous

* **main:** release 0.3.0 ([5cabeba](https://github.com/bluegardenproject/tracks/commit/5cabebaf50d614507f7095769d50c7a9e4635943))

## [0.3.0](https://github.com/bluegardenproject/tracks/compare/v0.2.0...v0.3.0) (2026-07-21)


### Features

* **claude:** add response-style and code-comment guidance to task prompt ([3022ceb](https://github.com/bluegardenproject/tracks/commit/3022ceb10d01fe905923360b45ab59db4ab6aa68))
* save a failed track creation as a draft ([793a777](https://github.com/bluegardenproject/tracks/commit/793a7771e7112b9b74c1696fc24dc46f284ff49d))
* **services:** run dev servers in a pane that owns the process ([466e05d](https://github.com/bluegardenproject/tracks/commit/466e05d58f9fc3eda5ad46b52f5ced8a92c2d611))
* **services:** start-all `tracks up` + live Proxy dashboard tab ([459281a](https://github.com/bluegardenproject/tracks/commit/459281aba51d4ddbea8555c9ca275a38b7de2cd1))


### Bug Fixes

* **dashboard:** keep track selection visible and stable ([487f703](https://github.com/bluegardenproject/tracks/commit/487f70376b12cfbc993b83ba8d087799f2fbffbc))
* **draft:** don't let End/Kill destroy a saved draft ([e5aa8de](https://github.com/bluegardenproject/tracks/commit/e5aa8de1aaef5ef5815b32cd29189459b611e2b8))
* never GC a worktree with unsaved work ([d551f80](https://github.com/bluegardenproject/tracks/commit/d551f800ed53e7adf50beefad25de287678920c5))
* **proxy:** bind stable-port proxy lazily so idle daemon frees the port ([3850624](https://github.com/bluegardenproject/tracks/commit/38506240fd9ed810c682b5ccd7be9a28767cb5b6))
* **services:** make the dev-server trigger reliable from inside a track ([b518a3a](https://github.com/bluegardenproject/tracks/commit/b518a3a7e6e22e9ac65502cebf819f31a21c1689))
* **test:** isolate StateDir in daemon server tests ([b3488e7](https://github.com/bluegardenproject/tracks/commit/b3488e7a0b5de95c1524facf991cb2f7b1267366))


### Documentation

* **roadmap:** reconcile with shipped work; record draft-on-failure ([88f2722](https://github.com/bluegardenproject/tracks/commit/88f27228edf25ffbc489c573911139f5aee5a90b))

## [0.2.0](https://github.com/bluegardenproject/tracks/compare/v0.1.0...v0.2.0) (2026-07-11)


### Features

* add install and uninstall scripts ([90df79d](https://github.com/bluegardenproject/tracks/commit/90df79d4814c33b2dec5855ad30b78386400ea64))
* **dashboard:** replace IDLE column with SVC, shrink BRANCH ([5aa0aef](https://github.com/bluegardenproject/tracks/commit/5aa0aef8b3f82ff44f8b5b1efc3c89d73a79ec92))
* resume finished tracks from their Claude session ([ac7ed74](https://github.com/bluegardenproject/tracks/commit/ac7ed74b8698024481df2b4fa0ba7b66fb59eb9f))
* **settings:** add per-repo submenu and service CRUD ([8622d01](https://github.com/bluegardenproject/tracks/commit/8622d014160728182d77fc565a6f850309d3466a))
* **status:** flip to PR as soon as PR URL appears in pane ([ddfe201](https://github.com/bluegardenproject/tracks/commit/ddfe20175b483d373190520f5d985a583f88ad79))


### Bug Fixes

* **dashboard:** make R (resume) keybinding async ([b68bdfd](https://github.com/bluegardenproject/tracks/commit/b68bdfd61dade2a7223a1e74acf8cc989094f496))
* resolve tmux pane, proxy menu, dep timing, and SVC column bugs ([b86548a](https://github.com/bluegardenproject/tracks/commit/b86548ae5441f845c5a5bc38f460431c6e259976))


### Documentation

* document curl install and tick roadmap distribution items ([eff7c80](https://github.com/bluegardenproject/tracks/commit/eff7c805d4981d494e96e1a1fab55a46c7278e19))
