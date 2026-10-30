# Cuenta de Microsoft y servidores online-mode

gocraft entra en servidores *online mode* igual que el juego original: inicia sesión con **tu propia
cuenta de Microsoft** (que debe tener Minecraft: Java Edition) y se lo demuestra al servidor con el
handshake cifrado de 1.8.9 y el servidor de sesión de Mojang. No falsea el cliente: el brand sigue
siendo `gocraft`. El diseño sigue el de Horizon (`docs/microsoft-account.md` de ese repo).

## Lo que tienes que hacer tú (una vez)

gocraft **no lleva ninguna aplicación de Azure incluida**, y nunca debe usarse el ID de otra
aplicación (el del launcher oficial o el de otro launcher). Hace falta una propia:

1. **Registrar la aplicación** en https://portal.azure.com → Microsoft Entra ID → App registrations →
   New registration:
   - Nombre: el que quieras (lo verás en la página de consentimiento de Microsoft).
   - Tipos de cuenta: **solo cuentas personales de Microsoft**.
   - Redirect URI: ninguna hace falta (flujo de código de dispositivo).
   - Authentication → Advanced settings → **Allow public client flows: Yes**.
   - Copia el **Application (client) ID** (un GUID). No es un secreto.
2. **Aprobación de Mojang**: una aplicación nueva recibe `403 Invalid app registration` en
   `api.minecraftservices.com` hasta que Mojang la aprueba. Solicítala en https://aka.ms/mce-reviewappid
   (puede tardar de días a semanas). Mientras tanto el inicio de sesión falla en el último paso con
   «Mojang has not approved this application yet».
3. **Poner el ID** en la variable de entorno `GOCRAFT_MSA_CLIENT_ID`, o en
   `~/.config/gocraft/options.txt` como `msaClientId:<GUID>`.

## Uso

Título → Multiplayer → **Sign in with Microsoft**: la pantalla muestra una dirección
(https://www.microsoft.com/link) y un código; introdúcelo en el navegador. Después:

- **Account: ON** juega con el nombre de la cuenta y la autentica ante servidores online-mode.
  **OFF** usa el nombre offline del campo (servidores offline, LAN, el servidor propio).
- **Sign out** borra la cuenta guardada.
- Un servidor online-mode sin sesión iniciada rechaza con «sign in with Microsoft on the Multiplayer screen».

## Dónde se guarda

- `~/.config/gocraft/account.json` (modo 0600): el *refresh token* de Microsoft, el UUID y el nombre.
  El token de acceso de Minecraft (24 h) vive solo en memoria. **Es un archivo**: Horizon guarda esto en el
  llavero del sistema, y gocraft aún no tiene forma de llegar a él. Cualquier proceso con tu usuario puede
  leerlo; bórralo con *Sign out* si compartes la máquina.
- Los tokens no se escriben en logs ni en mensajes de error.

## Flujo (para depurar)

device code → token de Microsoft → Xbox Live (`user.auth.xboxlive.com`) → XSTS
(`rp://api.minecraftservices.com/`) → `login_with_xbox` → entitlements (`game_minecraft`) → perfil.
Al unirse a un servidor: `POST sessionserver.mojang.com/session/minecraft/join` con el hash del
serverId, el secreto compartido y la clave pública del servidor, y AES/CFB8 con ese secreto.

Probado con servicios falsos (`auth/auth_test.go`), con el vector de NIST para CFB8
(`proto/cfb8_test.go`) y con un servidor online-mode falso con RSA y AES reales
(`client/online_test.go`). **No se ha probado contra los servicios reales de Microsoft ni de Mojang**:
eso exige tu ID de aplicación y su aprobación.
