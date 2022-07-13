# Tunveo

Project được fork từ open source cloudflared của cloudflare và sửa lại một số phần để có thể sử dụng giống như ngrok

## Expose local service to the internet


Download [Tunveo release](https://github.com/thuongnv-opensource/tunveo/releases)


install linux
```
rm -f /usr/bin/tunveo && wget https://github.com/thuongnv-opensource/tunveo/releases/download/v0.5/tunveo-linux-amd64 -O /usr/bin/tunveo && chmod +x /usr/bin/tunveo
```

install macos
```
rm -f /usr/local/bin/tunveo && curl -L https://github.com/thuongnv-opensource/tunveo/releases/download/v0.5/tunveo-darwin --output /usr/local/bin/tunveo && chmod +x /usr/local/bin/tunveo
```

**usage**
```
tunveo http --port 3000
```
