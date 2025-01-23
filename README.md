# Badge "Magic" Tool

## Download font

NOTE: This step is optional, getting you a nice compact font.

http://littlelimit.net/k8x12.htm

```sh
wget https://littlelimit.net/arc/k8x12/k8x12_ttf_2021-05-05.zip
unzip -d k8x12 k8x12_ttf_2021-05-05.zip
```

## Download prebuilt version

```sh
wget https://github.com/orangecms/fossasia-badge/releases/download/v0.0.11/badgemagic-tool
chmod +x badgemagic-tool # set executable permissions
```

## Run it

```sh
sudo ./badgemagic-tool -mode anim "Your text..."
```

NOTE: By default, the font is assumed to be in a subdirectory named `k8x12`.

With custom font:

```sh
sudo ./badgemagic-tool -mode anim -font path/to/font.ttf "Your text..."
```

NOTE: If you set up udev rules, you will not need `sudo`.
Copy the file `99-ledbadge.rules` to `/etc/udev/rules.d/`.

## Build yourelf

Given a Go compiler, after cloning this repository:

```sh
go build .
```
