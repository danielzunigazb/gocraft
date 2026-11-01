# gocraft

Minecraft 1.8.9 (protocolo 47) reescrito en Go: un **cliente** con OpenGL 3.3 y un **servidor**.
El cliente juega en mundos propios (formato Anvil de 1.8.9) y se conecta a servidores de terceros
(probado como objetivo en `mc.universocraft.com`).

No incluye ningún recurso de Mojang: las texturas, modelos y sonidos se leen del `client.jar` de
1.8.9 **que ya tienes** (`-jar`).

## Qué necesitas

- Go 1.26 o más nuevo.
- OpenGL 3.3 y GLFW (en Linux: los paquetes de desarrollo de X11/Wayland y GL de tu distribución).
- El `client.jar` de Minecraft 1.8.9 (por defecto `~/mc-1.8.9-run/client.jar`).

## Compilar y jugar

```sh
go build -o gocraft ./cmd/gocraft
./gocraft                                  # menú: un jugador, multijugador, opciones
./gocraft -connect mc.universocraft.com:25565 -name TuNombre
./gocraft -world ~/.minecraft/saves/Mundo  # abre un mundo 1.8.9
```

Los flags más útiles (`./gocraft -h` los lista todos):

| Flag | Para qué |
|---|---|
| `-connect host:port` | entrar directo a un servidor |
| `-name` | nombre en modo offline |
| `-jar`, `-options`, `-packs`, `-saves` | dónde están tu jar, `options.txt`, resource packs y mundos |
| `-netlog fichero` | vuelca cada paquete (dirección, id, tamaño, hora) para averiguar por qué un servidor rechaza al cliente |
| `-bench N` / `-cpuprofile f` | vuelo fijo de N segundos con estadísticas / perfil de CPU |
| `-nosound` | sin sonido |

Variables de entorno: `GOCRAFT_BRAND` (el brand que se anuncia; por defecto `gocraft`),
`GOCRAFT_MSA_CLIENT_ID` (ver `ACCOUNT.md`), `GOCRAFT_NO_AUDIO`, `GOCRAFT_PREVIEW`.

Servidores *online mode*: hace falta tu propia cuenta de Microsoft, ver [ACCOUNT.md](ACCOUNT.md).

## Capturas de pantalla y pruebas visuales

Sin ventana visible, para comprobar un cambio de dibujo:

```sh
xvfb-run -a ./gocraft -demo -screen netmobs -look 0,18 -screenshot /tmp/x.png
```

`-demo` pone bloques de ejemplo y `-screen` elige la escena: `netchat`, `netmenu`, `netfish`,
`netheld`, `netitems`, `netmobs`, `netparticles`, `netboss`, `cracks`, `signs`, `drag`,
`survival`, `inventory`, `title`... Siempre con `-demo` las que empiezan por `net`.

## Pruebas

```sh
go vet ./... && go test ./...
```

Unas 400 pruebas. Las del cliente necesitan el `client.jar` (si no está, se saltan). Ninguna toca un
servidor real: los paquetes se reproducen contra el cliente.

## Mapa del código

| Carpeta | Contenido |
|---|---|
| `cmd/gocraft` | el ejecutable |
| `client/` | el juego: render, física, mobs, interfaz y `net*.go` (multijugador) |
| `server/` | el servidor propio (mundo, jugadores, combate básico) |
| `proto/` | tramas del protocolo: VarInt, compresión, cifrado CFB8 |
| `auth/` | inicio de sesión con Microsoft y servidor de sesión |
| `anvil/`, `world/`, `worldgen/` | formato Anvil, el mundo en memoria y la generación |

Piezas del cliente que conviene conocer:

- `client/winclick.go`: el motor de clics de ventanas (`Container.slotClick` de 1.8.9). Lo usan el
  inventario, la mesa, cofres, hornos y las ventanas que abre un servidor; `winmap.go` lo une al
  almacenamiento del juego y `winui.go` es el ratón y el teclado (`GuiContainer`).
- `client/net.go`: lectura de paquetes (`parsePacketIn`) y estado de la conexión.
- `client/netmob.go`, `netitem.go`, `netparticles.go`, `xporbs.go`, `signs.go`, `cracks.go`,
  `bossbar.go`: lo que un servidor muestra (mobs, items, partículas, orbes, carteles...).

## Estado

El cliente entra y se queda en universocraft, con chat con colores, menús, scoreboard, tab, títulos,
skins, PvP, inventario con todos los modos de clic, mobs, items, partículas y más. Lo pendiente
está en [ROADMAP.md](ROADMAP.md).

Cuando algo se comporta distinto de vanilla en un servidor, lo más útil es grabar la sesión con
`-netlog` y compararla con la de un cliente real a través de un proxy que registre paquetes.
