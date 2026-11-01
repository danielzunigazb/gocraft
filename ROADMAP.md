# Roadmap

`[x]` hecho y con pruebas, `[ ]` pendiente. Lo marcado con ⚠ está hecho pero **sin probar en un servidor
real**: las pruebas reproducen paquetes contra el cliente, no hablan con universocraft.

## Entrada y movimiento
- [x] Entrar a universocraft (anti-bot: plataforma de nieve, fases de vuelo y caída)
- [x] Velocidades, sprint y abilities exactos (⚠)
- [ ] Comparar la traza de movimiento con la de vanilla (sprint, efectos, vuelo)

## El mundo en multijugador
- [x] Items en el suelo y su recogida
- [x] Mobs con modelo: creeper, esqueleto, araña, zombi, slime, pigman, araña de cueva, murciélago, bruja,
      cerdo, oveja, vaca, gallina, calamar, conejo, aldeano, enderman, blaze, golem de hierro, lobo
- [x] Partículas, efectos de mundo, explosiones, chispas de crítico
- [x] Grietas al picar, carteles, orbes de experiencia
- [x] Barra de jefe (Wither o Dragón falsos), tapas de cofre, notas de note block
- [x] Cuadros (paintings): las 26 obras de EnumArt, posición, luz por cuadro y caja de clic
- [x] Item frames: modelo del bloque, objeto con su giro y tamaño, y el mapa que lleven
- [x] Rayos (0x2C): forma exacta de RenderLightningBolt, trueno, golpes repetidos y destello del cielo
- [x] Correas (0x1B, leash 1): la cuerda de RenderLiving con su caída y el nudo (objeto 77)
- [ ] Monturas (0x1B, leash 0) y caballos
- [x] Cabezas (calaveras de esqueleto, wither, zombi, jugador con su skin y creeper) como bloque, por 0x35
- [x] Spawners (0x35, acción 1): el mob gira en la jaula como MobSpawnerBaseLogic y TileEntityMobSpawnerRenderer
- [ ] Banners (0x35, acción 6); pistones
- [x] Armor stands visibles: modelo exacto, pose, brazos, plato, pequeños, armadura y objeto en la mano
- [x] Calaveras sobre la cabeza de un armor stand, a 1.1875 de tamaño
- [ ] Bloques y objetos que no son calaveras sobre la cabeza de un armor stand o de un jugador
- [x] Equipo de los mobs bípedos (armadura en sus dos tamaños, delgada en el esqueleto, y objeto en la mano)
- [ ] Cabeza grande de los bebés
- [x] Modelos de ghast (tentáculos de Random(1660), ojos al disparar) y magma cube (láminas y núcleo)
- [x] Snowman (ModelSnowMan y la calabaza de LayerSnowmanHead)
- [x] Silverfish, endermite, gigante (zombi a escala 6) y mooshroom (sin los champiñones del lomo todavía)
- [ ] Modelos: caballo, ocelote, guardián, wither, dragón
- [x] Borde del mundo (0x44), dibujado con mezcla alfa normal (la aditiva de vanilla dio colores erróneos en headless)
- [x] Mapas (0x34): paleta MapColor, rectángulos de colores y el mapa dibujado en un item frame
- [ ] Mapa en la mano y marcadores de los mapas; remolino de poción lanzada

## Ventanas e inventario
- [x] Motor de clics único (modos 0-6) e inventario por servidores ajenos (⚠)
- [x] Ventanas del servidor: cofre, hopper, dispenser, horno y mesa de crafteo, con predicción (⚠)
- [x] Arrastrar para repartir, doble clic, teclas 1-9, Q, clic central, vista previa
- [x] Mesa de encantamientos (botones, niveles, pistas, Enchant Item), yunque (nombre, coste, MC|ItemName) y alquimia (flecha y burbujas)
- [ ] Faro, aldeano y caballo (hoy se dibujan como filas de cofre)
- [x] La mesa de crafteo de un servidor vanilla llega con 0 casillas en el paquete: el tamaño sale ahora del tipo
- [ ] Inventario creativo por la misma vía

## Interfaz
- [x] Scoreboard (lista con números o corazones, lateral y bajo el nombre), tab con cabeceras, títulos, skins
- [x] Autocompletado con Tab
- [x] Cursiva dibujada, como FontRenderer: arriba +1 y abajo -1 píxel
- [x] Editor de carteles (0x36 y 0x12): cuatro líneas, flechas, Enter, borrar, ancho de 90 px, Hecho y Escape
- [ ] Estadísticas (0x37)
- [x] Resource packs del servidor (0x48): pregunta Sí/No como GuiYesNo, descarga con las cabeceras de vanilla (50 MiB, SHA-1),
      caché en `server-resource-packs/<hash>`, se pone sobre los del jugador y se quita al salir; la respuesta se recuerda por servidor (⚠ sin probar en un servidor real)

## Servidor propio
- [x] Comandos básicos: help, list, say, me, msg, tp, time, give, kill, seed y save-all
- [ ] Ventanas e inventarios reales (reutilizando el motor de clics)
- [ ] Mobs del lado del servidor
- [x] Guardar el mundo y los jugadores (`-save`): columnas editadas como mundo 1.8.9 y datos de cada jugador, que vuelve donde estaba

## Calidad
- [x] README y ROADMAP
- [ ] Pruebas que reproduzcan una captura de vanilla contra el cliente
- [x] Pasada de rendimiento con el profiler, por el lado de la CPU (2026-11-01, `-bench` con `-cpuprofile` bajo xvfb):
      el 79 % es el rasterizador por software de Mesa; del código de Go solo destaca el mallado (~6 ms por columna en un
      trabajador, 12 %), el *streaming* cuesta 0.04 ms por frame y el resto no pasa de 1 %. No hay cuello de botella que
      arreglar en la CPU
- [ ] Pasada de rendimiento en una GPU real (`tools/bench.sh` en tu pantalla: aquí no hay GPU)
- [x] Build distribuible: `make release` (el juego enlaza GLFW por cgo, así que se compila en la plataforma de destino)
