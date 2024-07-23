package main

import (
	"log"
	"math"
	"time"
)

type PIDMode int64

const (
	Manual		PIDMode = iota
	Automatic
	Timer
)

type PIDDirection int64

const (
	Direct		PIDDirection = iota
	Reverse
)

type QuickPID struct {
	AutoTune				*AutoTunePID

	dispKp 					float64
	dispKi 					float64
	dispKd 					float64

	peTerm 					float64
	pmTerm 					float64
	iTerm 					float64
	deTerm 					float64
	dmTerm					float64

	kP 						float64
	kI 						float64
	kD 						float64

	pOn 					float64
	dOn 					float64
	kPe 					float64
	kPm 					float64
	kDe 					float64
	kDm 					float64

	input					*float64
	output 					*float64
	setpoint 				*float64

	mode 					PIDMode
	controllerDirection 	PIDDirection
	sampleTime  			time.Duration
	lastTime 				time.Time
	outMin 					float64
	outMax 					float64
	error 					float64
	outputSum 				float64
	lastInput 				float64
	inAuto 					bool
}

func NewQuickPIDExplicit(input *float64, output *float64, setpoint *float64, kp float64, ki float64, kd float64, direction PIDDirection, pOn float64, dOn float64) *QuickPID {
	p := &QuickPID {
		output: output,
		input: input,
		setpoint: setpoint,
		mode: Manual,
	}

	p.SetOutputLimits(0.0, 255.0)
	p.sampleTime = time.Duration(0.1 * float64(time.Second))
	p.SetControllerDirection(direction)
	p.SetTuningsOn(kp, ki, kd, pOn, dOn)

	p.lastTime = time.Now().Add(-p.sampleTime)

	return p
}

func NewQuickPIDDirection(input *float64, output *float64, setpoint *float64, kp float64, ki float64, kd float64, direction PIDDirection) *QuickPID {
	return NewQuickPIDExplicit(input, output, setpoint, kp, ki, kd, direction, 1.0, 0.0)
}

func NewQuickPID(input *float64, output *float64, setpoint *float64, kp float64, ki float64, kd float64) *QuickPID {
	return NewQuickPIDDirection(input, output, setpoint, kp, ki, kd, Direct)
}

func (p *QuickPID) Compute() bool {
	if p.mode == Manual {
		return false
	}

	now := time.Now()
	timeChange := now.Sub(p.lastTime)

	if (p.mode == Timer || timeChange >= p.sampleTime) {
		input := *p.input
		dInput := input - p.lastInput
		p.error = *p.setpoint - input

		if (p.controllerDirection == Reverse) {
			p.error = -p.error
			dInput = -dInput
		}

		p.pmTerm = p.kPm * dInput
		p.peTerm = p.kPe * p.error

		p.outputSum += p.iTerm

		if (p.outputSum > p.outMax) {
			p.outputSum -= p.outputSum - p.outMax
		} else if p.outputSum < p.outMin {
			p.outputSum += p.outMin - p.outputSum
		} 

		p.outputSum = constrain(p.outputSum, p.outMin, p.outMax)
		p.outputSum = constrain(p.outputSum - p.pmTerm, p.outMin, p.outMax)
		*p.output = constrain(p.outputSum + p.peTerm + p.dmTerm - p.deTerm, p.outMin, p.outMax)

		p.lastInput = input
		p.lastTime = now
		return true
	} else {
		return false
	}
}

func (p *QuickPID) SetTuningsOn(kp float64, ki float64, kd float64, pOn float64, dOn float64) {
	if kp < 0 || ki < 0 || kd < 0 {
		return
	}

	p.pOn = pOn
	p.dOn = dOn

	p.dispKp = kp
	p.dispKi = ki
	p.dispKd = kd

	p.kP = kp
	p.kI = ki * p.sampleTime.Seconds()
	p.kD = kd / p.sampleTime.Seconds()

	p.kPe = p.kP * p.pOn
	p.kPm = p.kP * (1.0 - p.pOn)
	p.kDe = p.kD * p.dOn
	p.kDm = p.kD * (1.0 - p.dOn)
}

func (p *QuickPID) SetTunings(kp float64, ki float64, kd float64) {
	p.SetTuningsOn(kp, ki, kd, 1.0, 0.0)
}

func (p *QuickPID) SetSampleTime(sampleTime time.Duration) {
	if sampleTime.Seconds() > 0 {
		ratio := sampleTime.Seconds() / p.sampleTime.Seconds()
		p.kI *= ratio
		p.kD /= ratio
		p.sampleTime = sampleTime
	}
}

func (p *QuickPID) SetOutputLimits(min float64, max float64) {
	if min >= max {
		return
	}
	p.outMin = min
	p.outMax = max

	if p.mode != Manual {
		*p.output = constrain(*p.output, min, max)
		p.outputSum = constrain(p.outputSum, min, max)
	}
}

func (p *QuickPID) SetMode(mode PIDMode) {
	if p.mode != Manual && mode != Manual {
		p.Initialize()
	}
	p.mode = mode
}

func (p *QuickPID) Initialize() {
	p.outputSum = *p.output
	p.lastInput = *p.input
	p.outputSum = constrain(p.outputSum, p.outMin, p.outMax)
}

func (p *QuickPID) SetControllerDirection(direction PIDDirection) {
	p.controllerDirection = direction
}

func (p *QuickPID) GetKp() float64 {
	return p.dispKp
}

func (p *QuickPID) GetKi() float64 {
	return p.dispKi
}

func (p *QuickPID) GetKd() float64 {
	return p.dispKd
}

func (p *QuickPID) GetPterm() float64 {
	return p.peTerm
}

func (p *QuickPID) GetIterm() float64 {
	return p.iTerm
}

func (p *QuickPID) GetDterm() float64 {
	return p.deTerm
}

func (p *QuickPID) GetMode() PIDMode {
	return p.mode
}

func (p *QuickPID) GetDirection() PIDDirection {
	return p.controllerDirection
}

func (p *QuickPID) NewAutoTune(tuningMethod TuningMethod) {
	p.AutoTune = NewAutoTunePID(p.input, p.output, tuningMethod)
}

func (p *QuickPID) ClearAutoTune() {
	if p.AutoTune != nil {
		p.AutoTune = nil
	}
}

type TuningMethod int64

const (
	ZieglerNicholsPI	TuningMethod = iota
	ZieglerNicholsPID
	TyreusLuybenPI
	TyreusLuybenPID
	CianconeMarlinPI
	CianconeMarlinPID
	AmigofPID
	PessenIntegralPID
	SomeOverShootPID
	NoOvershootPID
)

type AutoTuneStage byte

const (
	AutoTune 			AutoTuneStage = iota
	Wait
	Stabilizing
	Coarse
	Fine
	Test
	T0
	T1
	T2
	T3L
	T3H
	Calc
	Tunings
	Clr
)

type AutoRuleConstant struct {
	ckp float64
	cki float64
	ckd float64
}

var AutoRulesConstants map[TuningMethod]AutoRuleConstant = map[TuningMethod]AutoRuleConstant {
   					   // ckp,      cki,       ckd x 1000
    ZieglerNicholsPI: 	{ ckp: 450, cki:  540, ckd:   0 },
	ZieglerNicholsPID: 	{ ckp: 600, cki:  176, ckd:  75 },
	TyreusLuybenPI: 	{ ckp: 313, cki:  142, ckd:   0 },
	TyreusLuybenPID: 	{ ckp: 454, cki:  206, ckd:  72 },
	CianconeMarlinPI: 	{ ckp: 303, cki: 1212, ckd:   0 },
	CianconeMarlinPID: 	{ ckp: 303, cki: 1333, ckd:  37 },
	AmigofPID: 			{ ckp:   0, cki:    0, ckd:   0 },
	PessenIntegralPID: 	{ ckp: 700, cki: 1750, ckd: 105 },
	SomeOverShootPID: 	{ ckp: 333, cki:  667, ckd: 111 },
	NoOvershootPID: 	{ ckp: 333, cki:  100, ckd:  67 },
}

type AutoTunePID struct {
	input 				*float64
	output 				*float64
	// setpoint 			*float64

	autoTuneStage 		AutoTuneStage
	tuningMethod 		TuningMethod
	controllerDirection PIDDirection
	printOrPlotter		bool
	tLoop 				time.Duration
	tLast 				time.Time
	t0 					time.Time
	t1 					time.Time
	t2 					time.Time
	t3 					time.Time

	outputStep 			float64
	hysteresis 			float64
	atSetpoint 			float64
	atOutput 			float64

	kU 					float64
	tU 					time.Duration
	tD 					time.Duration
	kP 					float64
	kI 					float64
	kD 					float64
	rdAvg 				float64
	peakHigh 			float64
	peakLow 			float64
	inputLast 			float64
}

func NewAutoTunePIDEmpty() *AutoTunePID {
	at := &AutoTunePID{
		input: nil,
		output: nil,
		autoTuneStage: Wait,
	}
	at.Reset()
	return at
}

func NewAutoTunePID(input *float64, output *float64, tuningMethod TuningMethod) *AutoTunePID {
	p := NewAutoTunePIDEmpty()
	p.input = input
	p.output = output
	p.tuningMethod = tuningMethod

	return p
}

func (p *AutoTunePID) Reset() {
	p.tLast = time.Time{}
	p.t0 = time.Time{}
	p.t1 = time.Time{}
	p.t2 = time.Time{}
	p.t3 = time.Time{}
	p.kU = 0.0
	p.tU = 0.0
	p.tD = 0.0
	p.kP = 0.0
	p.kI = 0.0
	p.kD = 0.0
	p.rdAvg = 0.0
	p.peakHigh = 0.0
	p.peakLow = 0.0
	p.autoTuneStage = AutoTune
}

func (p *AutoTunePID) AutoTuneConfig(outputStep float64, hysteresis float64, atSetpoint float64, atOutput float64, 
									 direction PIDDirection, printOrPlotter bool, sampleTime time.Duration) {
	p.outputStep = outputStep
	p.hysteresis = hysteresis
	p.atSetpoint = atSetpoint
	p.atOutput = atOutput
	p.controllerDirection = direction
	p.printOrPlotter = printOrPlotter
	p.tLoop = time.Duration(constrain(float64((sampleTime / 8.0)), 500.0, 16383.0))
	p.tLast = time.Now().Add(-p.tLoop)
	p.autoTuneStage = Stabilizing
}

func (p *AutoTunePID) AutoTuneLoop() AutoTuneStage {
	if (time.Since(p.tLast) <= p.tLoop) {
		return Wait
	} else {
		p.tLast = time.Now()
	}

	switch p.autoTuneStage {
		case AutoTune:
			return AutoTune
		case Wait:
			return Wait
		case Stabilizing:
			if p.printOrPlotter {
				log.Default().Println("Stabilizing →")
			}
			p.t0 = time.Now()
			p.peakHigh = p.atSetpoint
			if p.controllerDirection == Reverse {
				*p.output = 0
			} else {
				*p.output = p.atOutput + (p.outputStep * 2)
			}
			p.autoTuneStage = Coarse
			return AutoTune
		case Coarse:
			if time.Since(p.t0) < 2000 * time.Millisecond {
				return AutoTune
			}
			if *p.input < (p.atSetpoint - p.hysteresis) {
				if p.controllerDirection == Reverse {
					*p.output = p.atOutput + (p.outputStep * 2)
				} else {
					*p.output = p.atOutput - (p.outputStep * 2)
				}
				p.autoTuneStage = Fine
			}
			return AutoTune
		case Fine:
			if *p.input > p.atSetpoint {
				if p.controllerDirection == Reverse {
					*p.output = p.atOutput - p.outputStep
				} else {
					*p.output = p.atOutput + p.outputStep
				}
				p.autoTuneStage = Test
			}
			return AutoTune
		case Test:
			if *p.input < p.atSetpoint {
				if p.printOrPlotter {
					log.Default().Println("AutoTune →")
				}
				if p.controllerDirection == Reverse {
					*p.output = p.atOutput + p.outputStep
				} else {
					*p.output = p.atOutput - p.outputStep
				}
				p.autoTuneStage = T0
			}
			return AutoTune
		case T0:
			if *p.input > p.atSetpoint {
				p.t0 = time.Now()
				if p.printOrPlotter {
					log.Default().Println("T0 →")
				}
				p.inputLast = *p.input
				p.autoTuneStage = T1
			}
			return AutoTune
		case T1:
			if *p.input > p.atSetpoint && *p.input > p.inputLast {
				p.t1 = time.Now()
				if p.printOrPlotter {
					log.Default().Println("T1 →")
				}
				p.autoTuneStage = T2
			}
			return AutoTune
		case T2:
			p.rdAvg = *p.input
			if p.rdAvg > p.peakHigh {
				p.peakHigh = p.rdAvg
			}
			if p.rdAvg < p.peakLow && p.peakHigh >= p.atSetpoint + p.hysteresis {
				p.peakLow = p.rdAvg
			}
			if p.rdAvg > p.atSetpoint + p.hysteresis {
				p.t2 = time.Now()
				if p.printOrPlotter {
					log.Default().Println("T2 →")
				}
				if p.controllerDirection == Reverse {
					*p.output = p.atOutput - p.outputStep
				} else {
					*p.output = p.atOutput + p.outputStep
				}
				p.autoTuneStage = T3L
			}
			return AutoTune
		case T3L:
			p.rdAvg = *p.input
			if p.rdAvg > p.peakHigh {
				p.peakHigh = p.rdAvg
			}
			if p.rdAvg < p.peakLow && p.peakHigh >= p.atSetpoint + p.hysteresis {
				p.peakLow = p.rdAvg
			}
			if p.rdAvg < p.atSetpoint - p.hysteresis {
				if p.controllerDirection == Reverse {
					*p.output = p.atOutput + p.outputStep
				} else {
					*p.output = p.atOutput - p.outputStep
				}
				p.autoTuneStage = T3H
			}
			return AutoTune
		case T3H:
			p.rdAvg = *p.input
			if p.rdAvg > p.peakHigh {
				p.peakHigh = p.rdAvg
			}
			if p.rdAvg < p.peakLow && p.peakHigh >= p.atSetpoint + p.hysteresis {
				p.peakLow = p.rdAvg
			}
			if p.rdAvg > p.atSetpoint + p.hysteresis {
				p.t3 = time.Now()
				if p.printOrPlotter {
					log.Default().Println("T3H → done")
				}
				p.autoTuneStage = Calc
			}
			return AutoTune
		case Calc:
			p.tD = p.t1.Sub(p.t0)
			p.tU = p.t3.Sub(p.t2)
			p.kU = (4.0 * p.outputStep * 2) / (math.Pi * math.Sqrt(math.Pow(p.peakHigh - p.peakLow, 2) - math.Pow(p.hysteresis, 2)))
			if p.tuningMethod == AmigofPID {
				if p.tD < time.Duration(0.1 * float64(time.Second)) {
					p.tD = time.Duration(0.1 * float64(time.Second))
				}
				p.kP = (0.2 + 0.45 * (float64(p.tU) / float64(p.tD))) / p.kU

				tI := (0.4 * float64(p.tD) + 0.8 * float64(p.tU)) / ((float64(p.tD) + 0.1 * float64(p.tU)) * float64(p.tD))
				tD := time.Duration((0.5 * float64(p.tD) * float64(p.tU)) / (0.3 * float64(p.tD) + float64(p.tU)))
				p.kI = p.kP / tI
				p.kD = p.kP * float64(tD)
			} else {
				p.kP = AutoRulesConstants[p.tuningMethod].ckp / 1000.0 * p.kU
				p.kI = AutoRulesConstants[p.tuningMethod].cki / 1000.0 * (p.kU / p.tU.Seconds())
				p.kD = AutoRulesConstants[p.tuningMethod].ckd / 1000.0 * (p.kU * p.tU.Seconds())
			}
			if p.printOrPlotter {
				// Controllability https://blog.opticontrols.com/wp-content/uploads/2011/06/td-versus-tau.png
				if float64(p.tU / p.tD) + 0.0001 > 0.75 {
					log.Default().Println("Process is easy to control.")
				} else if float64(p.tU / p.tD) + 0.0001 > 0.25 {
					log.Default().Println("Process has average controllability.")
				} else {
					log.Default().Println("Process is difficult to control.")
				}
				log.Default().Println("tU: ", p.tU.Seconds()) // Ultimate Period (sec)
				log.Default().Println("tD: ", p.tD.Seconds()) // Dead Time (sec)
				log.Default().Println("kU: ", p.kU) // Ultimate Gain
				log.Default().Println("kP: ", p.kP) // Proportional Gain
				log.Default().Println("kI: ", p.kI) // Integral Gain
				log.Default().Println("kD: ", p.kD) // Derivative Gain
			}
			p.autoTuneStage = Tunings
			return AutoTune
		case Tunings:
			p.autoTuneStage = Clr
			return Tunings
		default:
			return Clr
	}
}

func (p *AutoTunePID) SetAutoTuneConstants(kp *float64, ki *float64, kd *float64) {
	*kp = p.kP
	*ki = p.kI
	*kd = p.kD
}


func constrain(val, min, max float64) float64 {
	if val < min {
		return min
	} else if val > max {
		return max
	}
	return val
}