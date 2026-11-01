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
- [x] Item frames: modelo del bloque, objeto con su giro y tamaño (los mapas dentro del marco aún no se dibujan)
- [ ] Rayos, correas y monturas
- [ ] Cabezas, banners y spawners (0x35); pistones
- [x] Armor stands visibles: modelo exacto, pose, brazos, plato, pequeños, armadura y objeto en la mano
- [ ] Cabezas y bloques sobre la cabeza de un armor stand (calaveras con skin)
- [ ] Equipo de los mobs, cabeza grande de los bebés
- [ ] Modelos: caballo, ocelote, ghast, guardián, magma cube, snowman
- [x] Borde del mundo (0x44), dibujado con mezcla alfa normal (la aditiva de vanilla dio colores erróneos en headless)
- [ ] Mapas (0x34), remolino de poción lanzada

## Ventanas e inventario
- [x] Motor de clics único (modos 0-6) e inventario por servidores ajenos (⚠)
- [x] Ventanas del servidor: cofre, hopper, dispenser, horno y mesa de crafteo, con predicción (⚠)
- [x] Arrastrar para repartir, doble clic, teclas 1-9, Q, clic central, vista previa
- [ ] Mesa de encantamientos, yunque, alquimia, faro, aldeano y caballo (hoy se dibujan como filas de cofre)
- [ ] Inventario creativo por la misma vía

## Interfaz
- [x] Scoreboard (lista con números o corazones, lateral y bajo el nombre), tab con cabeceras, títulos, skins
- [x] Autocompletado con Tab
- [ ] Cursiva dibujada (el parser ya la lee), editor de carteles (0x36), estadísticas (0x37)
- [ ] Resource packs: hoy se rechazan

## Servidor propio
- [ ] Ventanas e inventarios reales (reutilizando el motor de clics)
- [ ] Mobs del lado del servidor
- [ ] Guardar el mundo y los jugadores

## Calidad
- [x] README y ROADMAP
- [ ] Pruebas que reproduzcan una captura de vanilla contra el cliente
- [ ] Pasada de rendimiento con el profiler
- [ ] Build distribuible
