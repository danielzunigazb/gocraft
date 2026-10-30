package client

import (
	"math"
	"math/rand"
	"strconv"
	"time"
)

// The player's status bars, as GuiIngame.renderPlayerStats and renderExpBar draw them: armour above the
// hearts, the hearts with their blink on a blow, absorption in gold, the shake at low health, the wave of
// regeneration, the poisoned and withered colours, the hunger bar, the air bubbles and the experience bar.

const (
	icoHeartContainer = 16
	icoHeartFull      = 36 // added to the colour's base: 16 normal, 52 poison, 88 wither
)

// hudState is what the status bars remember between frames.
type hudState struct {
	ticks      int       // updateCounter: ticks the game has run
	shownHP    int       // playerHealth: the health at the last frame
	lastHP     int       // lastPlayerHealth: what the blink shows the loss against
	blinkUntil int       // healthUpdateCounter
	changedAt  time.Time // lastSystemTime
}

func (g *game) drawStatus(s, x0, y0 float32) {
	hd := &g.hud
	st := &g.statusBars
	one := [4]float32{1, 1, 1, 1}
	w, h := float32(g.fb[0]), float32(g.fb[1])
	left, right, k1 := w/2-91*s, w/2+91*s, h-39*s
	blit := func(x, y float32, u, v float32) { hd.rect(x, y, 9*s, 9*s, u, v, 9, 9, 256, 256, one) }

	hp := int(math.Ceil(float64(g.health)))
	blink := st.blinkUntil > st.ticks && (st.blinkUntil-st.ticks)/3%2 == 1
	switch {
	case hp < st.shownHP && g.hurtResist > 0:
		st.changedAt, st.blinkUntil = time.Now(), st.ticks+20
	case hp > st.shownHP && g.hurtResist > 0:
		st.changedAt, st.blinkUntil = time.Now(), st.ticks+10
	}
	if time.Since(st.changedAt) > time.Second {
		st.shownHP, st.lastHP = hp, hp
		st.changedAt = time.Now()
	}
	prevShown := st.lastHP
	st.shownHP = hp
	_ = prevShown
	j := st.lastHP

	maxHP := g.maxHP
	if maxHP <= 0 {
		maxHP = maxHealth
	}
	abs := g.absorption
	rows := int(math.Ceil(float64((maxHP + abs) / 2 / 10)))
	space := max(10-(rows-2), 3)
	armourY := k1 - float32((rows-1)*space+10)*s
	regenAt := -1
	if _, ok := g.effects[effRegen]; ok {
		regenAt = st.ticks % int(math.Ceil(float64(maxHP+5)))
	}

	// the armour
	if a := g.armourTotal(); a > 0 {
		for i := 0; i < 10; i++ {
			x := left + float32(i*8)*s
			switch {
			case i*2+1 < a:
				blit(x, armourY, 34, 9)
			case i*2+1 == a:
				blit(x, armourY, 25, 9)
			default:
				blit(x, armourY, 16, 9)
			}
		}
	}

	// the hearts, from the top one down, so that gold absorption is drawn from the top
	base := float32(16)
	if _, ok := g.effects[effPoison]; ok {
		base += 36
	} else if _, ok := g.effects[effWither]; ok {
		base += 72
	}
	f2 := abs
	for i := int(math.Ceil(float64((maxHP+abs)/2))) - 1; i >= 0; i-- {
		k3 := float32(0)
		if blink {
			k3 = 1
		}
		layer := int(math.Ceil(float64(i+1)/10)) - 1
		x := left + float32(i%10*8)*s
		y := k1 - float32(layer*space)*s
		if hp <= 4 {
			y += float32(rand.Intn(2)) * s
		}
		if i == regenAt {
			y -= 2 * s
		}
		blit(x, y, icoHeartContainer+k3*9, 0)
		if blink {
			if i*2+1 < j {
				blit(x, y, base+54, 0)
			}
			if i*2+1 == j {
				blit(x, y, base+63, 0)
			}
		}
		if f2 <= 0 {
			if i*2+1 < hp {
				blit(x, y, base+36, 0)
			}
			if i*2+1 == hp {
				blit(x, y, base+45, 0)
			}
		} else {
			if f2 == abs && math.Mod(float64(abs), 2) == 1 {
				blit(x, y, base+153, 0)
			} else {
				blit(x, y, base+144, 0)
			}
			f2 -= 2
		}
	}

	// the hunger bar, shaking when the saturation has run out
	hungerBase, hungerBg := float32(16), float32(0)
	if _, ok := g.effects[17]; ok {
		hungerBase, hungerBg = 52, 13
	}
	for i := 0; i < 10; i++ {
		y := k1
		if g.saturation <= 0 && st.ticks%(g.food*3+1) == 0 {
			y += float32(rand.Intn(3)-1) * s
		}
		x := right - float32(i*8)*s - 9*s
		blit(x, y, 16+hungerBg*9, 27)
		if i*2+1 < g.food {
			blit(x, y, hungerBase+36, 27)
		}
		if i*2+1 == g.food {
			blit(x, y, hungerBase+45, 27)
		}
	}

	// the air, while the head is under water
	if g.fluidIn(false) && g.air < 300 {
		full := int(math.Ceil(float64(g.air-2) * 10 / 300))
		burst := int(math.Ceil(float64(g.air)*10/300)) - full
		for i := 0; i < full+burst; i++ {
			x := right - float32(i*8)*s - 9*s
			if i < full {
				blit(x, k1-10*s, 16, 18)
			} else {
				blit(x, k1-10*s, 25, 18)
			}
		}
	}
	hd.flush(hd.icons)

	// the experience bar and the level
	if g.xpBar > 0 || g.xpLevel > 0 {
		y := h - (32-3)*s
		hd.rect(left, y, 182*s, 5*s, 0, 64, 182, 5, 256, 256, one)
		if k := int(g.xpBar * 183); k > 0 {
			hd.rect(left, y, float32(k)*s, 5*s, 0, 69, float32(k), 5, 256, 256, one)
		}
		hd.flush(hd.icons)
	}
	if g.xpLevel > 0 {
		text := strconv.Itoa(g.xpLevel)
		x := (w - float32(g.textWidth(text))*s) / 2
		y := h - (31+4)*s
		black := [4]float32{0, 0, 0, 1}
		for _, d := range [][2]float32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			g.text(text, x+d[0]*s, y+d[1]*s, s, black, false)
		}
		g.text(text, x, y, s, [4]float32{0x80 / 255.0, 1, 0x20 / 255.0, 1}, false)
	}
}
