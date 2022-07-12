# Tunveo

## Expose local service to the internet


Download [Tunveo release](https://github.com/thuongnv-opensource/tunveo/releases)


install linux
```
rm -f /usr/bin/tunveo && wget https://github.com/thuongnv-opensource/tunveo/releases/download/v0.7/tunveo-linux-amd64 -O /usr/bin/tunveo && chmod +x /usr/bin/tunveo
```

install macos
```
rm -f /usr/local/bin/tunveo && curl -L https://github.com/thuongnv-opensource/tunveo/releases/download/v0.7/tunveo-darwin --output /usr/local/bin/tunveo && chmod +x /usr/local/bin/tunveo
```

window
```
Chưa test
```

**usage example**
```
tunveo http --port 3000 # Expose localhost port 3000 to the internet
```

<img width="1033" alt="image" src="https://user-images.githubusercontent.com/24992586/178136220-02bc412f-65fa-4cba-948e-8ae1fbbad9ca.png">
