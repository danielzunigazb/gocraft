package client

import "math"

// The player: ModelPlayer's boxes with the skin's second layer (hat, jacket, sleeves, trousers), the
// cape of ModelPlayer.renderCape and a pair of feathered wings of our own, drawn with the same
// box system as the mobs.

const (
	pHead = iota
	pHat
	pBody
	pJacket
	pRArm
	pRSleeve
	pLArm
	pLSleeve
	pRLeg
	pRPants
	pLLeg
	pLPants
)

var playerMdl, playerSlim, capeMdl, wingMdl *mmodel

func init() {
	defer func() { kinds["PvPBot"].model = playerMdl }() // the practice bot wears the same model
	playerMdl = newModel(64, 64,
		part(0, 0, 0, box(0, 0, -4, -8, -4, 8, 8, 8)),
		part(0, 0, 0, boxX(32, 0, -4, -8, -4, 8, 8, 8, 0.5)),
		part(0, 0, 0, box(16, 16, -4, 0, -2, 8, 12, 4)),
		part(0, 0, 0, boxX(16, 32, -4, 0, -2, 8, 12, 4, 0.25)),
		part(-5, 2, 0, box(40, 16, -3, -2, -2, 4, 12, 4)),
		part(-5, 2, 0, boxX(40, 32, -3, -2, -2, 4, 12, 4, 0.25)),
		part(5, 2, 0, box(32, 48, -1, -2, -2, 4, 12, 4)),
		part(5, 2, 0, boxX(48, 48, -1, -2, -2, 4, 12, 4, 0.25)),
		part(-1.9, 12, 0, box(0, 16, -2, 0, -2, 4, 12, 4)),
		part(-1.9, 12, 0, boxX(0, 32, -2, 0, -2, 4, 12, 4, 0.25)),
		part(1.9, 12, 0, box(16, 48, -2, 0, -2, 4, 12, 4)),
		part(1.9, 12, 0, boxX(0, 48, -2, 0, -2, 4, 12, 4, 0.25)),
	)
	// Alex's model: the same, with arms three wide that hang from half a unit lower.
	playerSlim = newModel(64, 64,
		part(0, 0, 0, box(0, 0, -4, -8, -4, 8, 8, 8)),
		part(0, 0, 0, boxX(32, 0, -4, -8, -4, 8, 8, 8, 0.5)),
		part(0, 0, 0, box(16, 16, -4, 0, -2, 8, 12, 4)),
		part(0, 0, 0, boxX(16, 32, -4, 0, -2, 8, 12, 4, 0.25)),
		part(-5, 2.5, 0, box(40, 16, -2, -2, -2, 3, 12, 4)),
		part(-5, 2.5, 0, boxX(40, 32, -2, -2, -2, 3, 12, 4, 0.25)),
		part(5, 2.5, 0, box(32, 48, -1, -2, -2, 3, 12, 4)),
		part(5, 2.5, 0, boxX(48, 48, -1, -2, -2, 3, 12, 4, 0.25)),
		part(-1.9, 12, 0, box(0, 16, -2, 0, -2, 4, 12, 4)),
		part(-1.9, 12, 0, boxX(0, 32, -2, 0, -2, 4, 12, 4, 0.25)),
		part(1.9, 12, 0, box(16, 48, -2, 0, -2, 4, 12, 4)),
		part(1.9, 12, 0, boxX(0, 48, -2, 0, -2, 4, 12, 4, 0.25)),
	)
	// The cape hangs from the back of the shoulders, turned so its picture faces out.
	capeMdl = newModel(64, 32, turned(part(0, 0, 2, box(0, 0, -5, 0, -1, 10, 16, 1)), 0, pi, 0))

	// Wings: six feathers a side, fanned from a root at the shoulder blade, each longer towards the middle.
	lengths := [6]float32{15, 17, 17, 15, 12, 9}
	var feathers []mpart
	for side := 0; side < 2; side++ {
		for i, l := range lengths {
			fan := (float32(i) - 1.5) * 0.30 // upper feathers rise, lower ones hang
			if side == 0 {                   // the right wing grows towards -x
				feathers = append(feathers, turned(part(-2.5, 3, 2.5, box(float32(i%4)*8, float32(i/4)*8, -l, -1, -0.5, l, 3, 1)), 0, 0.45, -fan))
			} else {
				feathers = append(feathers, turned(part(2.5, 3, 2.5, box(float32(i%4)*8, float32(i/4)*8, 0, -1, -0.5, l, 3, 1)), 0, -0.45, fan))
			}
		}
	}
	wingMdl = newModel(64, 32, feathers...)
}

// poseState is what a pose needs to know about whoever wears the model.
type poseState struct {
	ls, lsa, age float32 // walking cycle, its strength, age in ticks
	headYaw      float32 // degrees, relative to the body
	headPitch    float32
	sneaking     bool
	swing        float32 // 0..1 through a swing of the arm, -1 when not swinging
	airborne     bool
	speedFwd     float32 // how fast it moves along its facing, blocks per tick
	holding      bool    // a weapon or tool in hand: the arm is bent
	blocking     bool
	aiming       bool // a bow drawn: both arms out in front
}

// playerRots poses the twelve parts as ModelBiped.setRotationAngles does, plus the sneak, the
// swing of the arm and the arms' idle sway.
func playerRots(st poseState) []rot {
	r := make([]rot, 12)
	playerRotsInto(r, st)
	return r
}

// playerRotsInto is playerRots into a slice the caller owns (twelve long), so that drawing allocates nothing.
func playerRotsInto(r []rot, st poseState) {
	r[pHead] = rot{x: st.headPitch * deg, y: st.headYaw * deg}
	rArm := rot{x: cos32(st.ls*armSw+pi) * 2 * st.lsa * 0.5}
	lArm := rot{x: cos32(st.ls*armSw) * 2 * st.lsa * 0.5}
	rLeg := rot{x: cos32(st.ls*armSw) * 1.4 * st.lsa}
	lLeg := rot{x: cos32(st.ls*armSw+pi) * 1.4 * st.lsa}
	body := rot{}
	if st.blocking { // a sword held across the chest
		rArm.x = rArm.x*0.5 - pi*0.3
		rArm.y = -pi / 6
	} else if st.holding { // anything in the hand bends the arm a little forward (heldItemRight)
		rArm.x = rArm.x*0.5 - pi/10
	}
	if st.swing >= 0 { // the arm swings down across the body
		sw := st.swing
		body.y = sin32(float32(math.Sqrt(float64(sw)))*pi*2) * 0.2
		rArm.y += body.y
		f := 1 - sw
		f = 1 - f*f*f
		f2 := sin32(f * pi)
		f3 := sin32(sw*pi) * -(r[pHead].x - 0.7) * 0.75
		rArm.x -= f2*1.2 + f3
		rArm.y += body.y * 2
		rArm.z += sin32(sw*pi) * -0.4
	}
	if st.sneaking {
		body.x = 0.5
		rArm.x += 0.4
		lArm.x += 0.4
		rLeg.dz, lLeg.dz = 4, 4
		rLeg.dy, lLeg.dy = -3, -3
		r[pHead].dy = 1
	}
	sway := cos32(st.age*0.09)*0.05 + 0.05
	rArm.z += sway
	lArm.z -= sway
	rArm.x += sin32(st.age*0.067) * 0.05
	lArm.x -= sin32(st.age*0.067) * 0.05
	if st.aiming { // ModelBiped.aimedBow
		hx, hy := r[pHead].x, r[pHead].y
		rArm.z, lArm.z = 0, 0
		rArm.y, lArm.y = -0.1+hy, 0.1+hy+0.4
		rArm.x, lArm.x = -pi/2+hx, -pi/2+hx
		rArm.z += sway
		lArm.z -= sway
		rArm.x += sin32(st.age*0.067) * 0.05
		lArm.x -= sin32(st.age*0.067) * 0.05
	}
	r[pHat] = r[pHead]
	r[pBody], r[pJacket] = body, body
	r[pRArm], r[pRSleeve] = rArm, rArm
	r[pLArm], r[pLSleeve] = lArm, lArm
	r[pRLeg], r[pRPants] = rLeg, rLeg
	r[pLLeg], r[pLPants] = lLeg, lLeg
	if st.sneaking { // the arms and legs hang from a body that has moved forward and down
		for _, i := range []int{pRArm, pRSleeve, pLArm, pLSleeve} {
			r[i].dy, r[i].dz = 0.2, 0
		}
	}
}

// capeRot is how far the cape swings out behind: a little at rest, more the faster the wearer goes
// forward and with each step.
func capeRot(st poseState) rot {
	a := (6 + min(st.speedFwd*100, 28)/2 + sin32(st.ls*0.6662*2)*32*st.lsa*0.15) * deg
	if st.sneaking {
		a += 25 * deg
	}
	if st.airborne {
		a += 12 * deg
	}
	return rot{ox: a}
}

// wingRots flaps the wings: slowly and wide at rest, quickly when in the air.
func wingRots(st poseState) []rot {
	r := make([]rot, 12)
	amp, rate := float32(0.10), float32(0.09)
	if st.airborne {
		amp, rate = 0.55, 0.35
	}
	flap := sin32(st.age*rate) * amp
	fold := st.lsa * 0.12
	for i := 0; i < 6; i++ {
		r[i] = rot{z: -flap, y: -fold}
		r[6+i] = rot{z: flap, y: fold}
		if st.sneaking {
			r[i].x, r[6+i].x = 0.3, 0.3
		}
	}
	return r
}
