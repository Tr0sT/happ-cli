# happ-cli

Наш headless-форк [GenkaOk/happ-cli](https://github.com/GenkaOk/happ-cli), исходный автор — [aimuzov](https://github.com/aimuzov). Внутри Xray; GUI HApp и happd для работы не нужны.

Основная задача: найти рабочий профиль, запомнить его и восстановить связь при сбое. AUTO необязателен.

## Быстрый старт

```sh
nix build github:Tr0sT/happ-cli
./result/bin/happ --help
```

Либо сборка из checkout с Go >= 1.26.4: `CGO_ENABLED=0 go build -o happ ./cmd/happ`.

```sh
happ sub add --name primary --url-file /путь/к/приватному-файлу-ссылки
happ sub list
happ sub use primary
happ list
happ select --working --url https://api.github.com
happ connect --health-check --failover
happ check --url https://api.github.com
happ sub update --all
```

`select --working` проверяет профили через временный SOCKS-прокси и сохраняет первый, получивший HTTP 2xx от нужного сайта. Маршруты не меняются; на запрос отводится восемь секунд. Если все проверки неудачны, прежний выбор остаётся. Для ручного выбора: `happ select 'Germany'` или номер из списка. Уже запущенное соединение нужно перезапустить.

`--home DIR` задаёт каталог состояния; по умолчанию `~/.config/happ-cli`. `--url-file -` читает ссылку из stdin. Поддерживаются ссылки `happ://add/...` и переопределение User-Agent через `--ua`.

Выбор сохраняется при обновлении и перестановке профилей. Без `--failover` исчезнувший профиль вызывает явную ошибку. С `--failover` клиент перебирает остальные профили при устойчивом сбое, не меняя предпочтительный выбор. Три неудачные проверки подряд завершают соединение с ошибкой; systemd может его перезапустить.

При обновлении проверяется конфигурация Xray. Пустой ответ или потеря больше половины списка сохраняют старые данные и возвращают ошибку. Значительное сокращение можно разрешить явно через `--accept-shrink`. Проверка конфигурации не заменяет проверку связи.

## NixOS

Форк предоставляет пакет и `nixosModules.default`:

```nix
happ-cli.url = "github:Tr0sT/happ-cli";
happ-cli.inputs.nixpkgs.follows = "nixpkgs";

imports = [ inputs.happ-cli.nixosModules.default ];
services.happ-cli.enable = true;
services.happ-cli.mode = "tun";
services.happ-cli.checkURL = "https://api.github.com";
services.happ-cli.failover = true;
services.happ-cli.autoUpdate = true;
```

Это фрагменты для inputs и NixOS-модуля, не один готовый файл. Конфигурация применяется вручную.

Служба использует `/var/lib/happ-cli`, доступный root. Добавление подписки и выбор профиля для службы выполняются с `--home /var/lib/happ-cli` и обычным повышением прав. Модуль конфликтует со старой `happ-vpn.service`: одновременно включать два VPN не нужно.

```sh
happ service start
happ service stop
happ service restart
happ service status
happ service logs
happ --home /var/lib/happ-cli check --tun --url https://api.github.com
```

Команды используют права systemd; автоматического sudo нет. Таймер раз в шесть часов обновляет подписки и перезапускает активное соединение после успешного обновления. Остановленная вручную служба остаётся остановленной.

Режимы `tun` и `tun-direct` проверяют и SOCKS-прокси, и реальную передачу HTTP через интерфейс `thapp`. Сохраняются маршруты LAN, Docker, Cloudflare veth и ответы SSH с TCP-порта 22. При остановке собственные маршруты удаляются; `cleanup-tun` убирает помеченные остатки после аварии. Глобальный IPv6 блокируется на время туннеля.

Остановка VPN возвращает обычную сеть: kill switch здесь нет. DNS для локальной сети и подключения к VPN может идти по обычному каналу. Для SSH на нестандартном порту через публичную сеть заранее нужен обходной маршрут. Другие VPN и правила маршрутизации требуют отдельной проверки.

## Профили и секреты

Полные Xray JSON-профили сохраняют маршрутизацию, балансировщики, DNS и настройки транспорта. Клиент заменяет входящие слушатели, отключает предоставленные подпиской API/metrics и файловые логи, задаёт собственные socket marks. Поддерживаются также VLESS/VMess/Trojan/Shadowsocks share links. Hysteria2 share links пока только отображаются. При несовместимости с встроенной версией Xray возвращается ошибка.

`state.json` содержит секретные ссылки и ключи, `device.json` — постоянные HWID/UUID. Не публикуйте их. Для подписки с лимитом устройств перенесите известную идентичность до первого запроса. Повреждённый `device.json` не заменяется молча новой идентичностью. Команда `happ config` выводит конфигурацию с секретами.

Nix-пакет содержит geoip/geosite и системные инструменты. Для самостоятельного бинарника они нужны отдельно; путь к `.dat` задаётся `XRAY_LOCATION_ASSET`.

## Проверки

```sh
go test ./...
go vet ./...
bash tests/netns.sh
```

Последний тест проверяет оба TUN-режима с настоящим локальным зашифрованным прокси, маршрутами Docker/Cloudflare, блокировкой IPv6 и очисткой. Он работает в изолированных пространствах имён и не меняет сеть хоста. Реальный провайдер и маршруты VDS проверяются отдельно.

Подробности и ограничения — в [английском README](README.md). Полезный сопутствующий проект: [d4nom/happ-vpn-setup](https://github.com/d4nom/happ-vpn-setup).
