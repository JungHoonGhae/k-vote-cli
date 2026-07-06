import {
  AbsoluteFill,
  Easing,
  Img,
  Sequence,
  interpolate,
  staticFile,
  useCurrentFrame,
} from "remotion";
import { loadFont as loadSans } from "@remotion/google-fonts/IBMPlexSansKR";
import { loadFont as loadMono } from "@remotion/google-fonts/IBMPlexMono";

const sans = loadSans();
const mono = loadMono();

export const PROMO_FPS = 30;
export const PROMO_DURATION = 1625; // 54.2s

// 디자인 시스템 — 절제가 규칙이다.
const BG = "#060606";
const INK = "#F2F1EC"; // off-white
const DIM = "#77766F"; // muted gray
const FAINT = "#4A4945";
const ACCENT = "#39D0C3"; // 유일한 색. 정당색과 겹치지 않는 청록.

const easeOut = Easing.bezier(0.16, 1, 0.3, 1);

// ---------- primitives ----------

/** 느린 페이드 + 6px 상승. 이 영상의 유일한 등장 모션. */
const Rise: React.FC<{
  from: number;
  children: React.ReactNode;
  dimAfter?: number; // 이 프레임 이후 35% 로 물러남
  style?: React.CSSProperties;
}> = ({ from, children, dimAfter, style }) => {
  const frame = useCurrentFrame();
  const t = interpolate(frame, [from, from + 26], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: easeOut,
  });
  let opacity = t;
  if (dimAfter !== undefined) {
    opacity *= interpolate(frame, [dimAfter, dimAfter + 20], [1, 0.35], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    });
  }
  return (
    <div style={{ opacity, transform: `translateY(${(1 - t) * 6}px)`, ...style }}>
      {children}
    </div>
  );
};

const Center: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <AbsoluteFill
    style={{
      backgroundColor: BG,
      alignItems: "center",
      justifyContent: "center",
      fontFamily: sans.fontFamily,
    }}
  >
    {children}
  </AbsoluteFill>
);

const fadeOutAll = (frame: number, at: number) =>
  interpolate(frame, [at, at + 22], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

/** 두 문장 장면: 첫 줄 등장 → (선택) 물러남 → 둘째 줄 등장 → 함께 퇴장.
 * sub 는 둘째 줄 아래 낮은 목소리(작고 흐리게). */
const Statement: React.FC<{
  line1: string;
  line2: string;
  size: number;
  line2From: number;
  outAt: number;
  dimFirst?: boolean;
  sub?: string;
  subFrom?: number;
}> = ({ line1, line2, size, line2From, outAt, dimFirst, sub, subFrom }) => {
  const frame = useCurrentFrame();
  return (
    <Center>
      <div style={{ opacity: fadeOutAll(frame, outAt), textAlign: "center" }}>
        <Rise from={12} dimAfter={dimFirst ? line2From - 8 : undefined}>
          <div style={{ fontSize: size, fontWeight: 600, color: INK, letterSpacing: "-0.01em" }}>
            {line1}
          </div>
        </Rise>
        <Rise from={line2From} style={{ marginTop: size * 0.62 }}>
          <div style={{ fontSize: size, fontWeight: 600, color: INK, letterSpacing: "-0.01em" }}>
            {line2}
          </div>
        </Rise>
        {sub ? (
          <Rise from={subFrom ?? line2From + 34} style={{ marginTop: size * 0.8 }}>
            <div style={{ fontSize: size * 0.55, fontWeight: 500, color: DIM }}>{sub}</div>
          </Rise>
        ) : null}
      </div>
    </Center>
  );
};

// ---------- scenes ----------

/** S4: 답은 한 줄. 타이핑만. */
const CMD = "kvote nec corpus --normalize";
const SceneCommand: React.FC = () => {
  const frame = useCurrentFrame();
  const typed = Math.round(
    interpolate(frame, [20, 20 + CMD.length * 2], [0, CMD.length], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    })
  );
  const cursorOn = Math.floor(frame / 16) % 2 === 0;
  const promptIn = interpolate(frame, [6, 26], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: easeOut,
  });
  return (
    <Center>
      <div
        style={{
          opacity: promptIn * fadeOutAll(frame, 108),
          fontFamily: mono.fontFamily,
          fontSize: 58,
          color: INK,
          letterSpacing: "0.01em",
        }}
      >
        <span style={{ color: DIM }}>$ </span>
        {CMD.slice(0, typed)}
        <span
          style={{
            display: "inline-block",
            width: "0.62em",
            height: "1.15em",
            verticalAlign: "-0.22em",
            backgroundColor: cursorOn ? ACCENT : "transparent",
            marginLeft: 6,
          }}
        />
      </div>
    </Center>
  );
};

/** 카운트업 숫자 하나. 유일하게 허락된 화려함. */
const Counter: React.FC<{
  from: number;
  target: number;
  label: string;
  outAt?: number;
}> = ({ from, target, label, outAt }) => {
  const frame = useCurrentFrame();
  const p = interpolate(frame, [from, from + 72], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: easeOut,
  });
  const n = Math.round(target * p);
  let opacity = interpolate(frame, [from, from + 18], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  if (outAt !== undefined) {
    opacity *= interpolate(frame, [outAt, outAt + 18], [1, 0], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    });
  }
  return (
    <div style={{ opacity, textAlign: "center", position: "absolute" }}>
      <div
        style={{
          fontFamily: mono.fontFamily,
          fontVariantNumeric: "tabular-nums",
          fontSize: 168,
          fontWeight: 500,
          color: INK,
          letterSpacing: "-0.02em",
        }}
      >
        {n.toLocaleString("ko-KR")}
      </div>
      <div style={{ fontSize: 44, color: DIM, marginTop: 18, fontWeight: 500 }}>{label}</div>
    </div>
  );
};

/** S5: 실데이터 카운트업 두 번. */
const SceneNumbers: React.FC = () => {
  const frame = useCurrentFrame();
  const opacity = fadeOutAll(frame, 176);
  return (
    <Center>
      <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", opacity }}>
        <Counter from={8} target={4260} label="읍면동" outAt={86} />
        <Counter from={100} target={22564394} label="표" />
      </AbsoluteFill>
      <div
        style={{
          position: "absolute",
          bottom: 90,
          left: 0,
          right: 0,
          textAlign: "center",
          fontSize: 26,
          color: FAINT,
          opacity,
        }}
      >
        제8회 전국동시지방선거 시·도지사선거 개표결과 기준 · 예시로 든 선거에 다른 의미는 없습니다
      </div>
    </Center>
  );
};

/** AI 에이전트 로고 marquee. 로고는 전부 단색(오프화이트) 처리 — 미니멀 톤 유지. */
const AGENT_LOGOS = [
  "claude",
  "codex",
  "cursor",
  "googlegemini",
  "githubcopilot",
  "perplexity",
  "ollama",
  "deepseek",
  "qwen",
  "mistralai",
  "moonshotai",
  "opencode",
  "openclaw",
];

const SceneAgents: React.FC = () => {
  const frame = useCurrentFrame();
  const logoH = 54;
  const cell = 190; // 로고 셀 폭 (로고 + 간격)
  const rowW = AGENT_LOGOS.length * cell;
  const x = -((frame * 2.6) % rowW);
  const mask =
    "linear-gradient(90deg, transparent, black 14%, black 86%, transparent)";
  return (
    <Center>
      <div style={{ width: "100%", opacity: fadeOutAll(frame, 122) }}>
        <Rise from={8}>
          <div
            style={{
              textAlign: "center",
              fontSize: 58,
              fontWeight: 600,
              color: INK,
              letterSpacing: "-0.01em",
            }}
          >
            쓰던 AI 에이전트에, 그대로 연결됩니다.
          </div>
        </Rise>
        <Rise from={32} style={{ marginTop: 100 }}>
          <div
            style={{
              width: "100%",
              overflow: "hidden",
              WebkitMaskImage: mask,
              maskImage: mask,
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                width: rowW * 2,
                transform: `translateX(${x}px)`,
              }}
            >
              {[...AGENT_LOGOS, ...AGENT_LOGOS].map((name, i) => (
                <div
                  key={`${name}-${i}`}
                  style={{ width: cell, display: "flex", justifyContent: "center" }}
                >
                  <Img
                    src={staticFile(`logos/${name}.svg`)}
                    style={{
                      height: logoH,
                      filter: "brightness(0) invert(0.92)",
                      opacity: 0.82,
                    }}
                  />
                </div>
              ))}
            </div>
          </div>
        </Rise>
        <Rise from={56} style={{ marginTop: 90 }}>
          <div style={{ textAlign: "center", fontSize: 30, color: DIM }}>
            MCP 지원 — 별도 연동 코드 없이
          </div>
        </Rise>
      </div>
    </Center>
  );
};

/** S7: 정체성. 세 구절. */
const SceneCreed: React.FC = () => {
  const frame = useCurrentFrame();
  const words = ["누구나.", "같은 명령.", "같은 데이터."];
  return (
    <Center>
      <div style={{ display: "flex", gap: 44, opacity: fadeOutAll(frame, 81) }}>
        {words.map((w, i) => (
          <Rise key={w} from={10 + i * 22}>
            <div style={{ fontSize: 92, fontWeight: 600, color: INK, letterSpacing: "-0.01em" }}>
              {w}
            </div>
          </Rise>
        ))}
      </div>
    </Center>
  );
};

/** S8: 워드마크 + 리포지토리. */
const SceneEnd: React.FC = () => {
  const frame = useCurrentFrame();
  const cursorOn = Math.floor(frame / 16) % 2 === 0;
  return (
    <Center>
      <div style={{ textAlign: "center" }}>
        <Rise from={6}>
          <div
            style={{
              fontFamily: mono.fontFamily,
              fontSize: 148,
              fontWeight: 600,
              color: INK,
              letterSpacing: "-0.02em",
            }}
          >
            kvote
            <span
              style={{
                display: "inline-block",
                width: "0.55em",
                height: "1.02em",
                verticalAlign: "-0.13em",
                backgroundColor: cursorOn ? ACCENT : "transparent",
                marginLeft: 14,
              }}
            />
          </div>
        </Rise>
        <Rise from={34} style={{ marginTop: 44 }}>
          <div style={{ fontSize: 42, color: INK, fontWeight: 500 }}>
            한국 선거 공개 데이터, API 키 없이 한 명령으로
          </div>
        </Rise>
        <Rise from={58} style={{ marginTop: 60 }}>
          <div style={{ fontFamily: mono.fontFamily, fontSize: 30, color: DIM }}>
            github.com/JungHoonGhae/k-vote-cli
          </div>
        </Rise>
      </div>
    </Center>
  );
};

// ---------- composition ----------
// 내러티브: 불신의 시대 → 미지에서 싹트는 불신 → 공개돼 있지만 닿기 어려운 데이터
// → 개발자 한 사람의 고민 → 한 줄의 답 → 실데이터
// → 열려 있을수록 가능해지는 신뢰(막연한 생각) → 정체성.

export const Promo: React.FC = () => {
  return (
    <AbsoluteFill style={{ backgroundColor: BG }}>
      <Sequence from={0} durationInFrames={135}>
        <Statement
          line1="선거관리에 대한 불신이"
          line2="어느 때보다 높아진 시대입니다."
          size={80}
          line2From={50}
          outAt={107}
        />
      </Sequence>
      <Sequence from={135} durationInFrames={140}>
        <Statement
          line1="미지의 영역에는"
          line2="두려움과 불신이 싹트기 마련이니까요."
          size={78}
          line2From={48}
          outAt={112}
        />
      </Sequence>
      <Sequence from={275} durationInFrames={175}>
        <Statement
          line1="개표결과도, 여론조사도 이미 공개되어 있습니다."
          line2="그러나 공개와 접근은, 다른 문제였습니다."
          size={70}
          line2From={74}
          outAt={147}
          dimFirst
          sub="막힌 포털, 낡은 인코딩, 제각각의 파일 형식, 일일이 신청해야 하는 API 키니까요."
          subFrom={110}
        />
      </Sequence>
      <Sequence from={450} durationInFrames={140}>
        <Statement
          line1="무엇이든 AI와 함께 분석하는 시대에,"
          line2="선거 데이터만은 아직 닿기 어려운 곳에 있습니다."
          size={66}
          line2From={52}
          outAt={112}
        />
      </Sequence>
      <Sequence from={590} durationInFrames={145}>
        <Statement
          line1="개발자의 한 사람으로서, 이 문제를 해결하고 싶었습니다."
          line2="이미 있는 데이터라도, 조금 더 닿기 쉽도록."
          size={60}
          line2From={70}
          outAt={117}
          dimFirst
        />
      </Sequence>
      <Sequence from={735} durationInFrames={135}>
        <SceneCommand />
      </Sequence>
      <Sequence from={870} durationInFrames={200}>
        <SceneNumbers />
      </Sequence>
      <Sequence from={1070} durationInFrames={150}>
        <SceneAgents />
      </Sequence>
      <Sequence from={1220} durationInFrames={165}>
        <Statement
          line1="더 많은 사람이 열어 보고 직접 검증할 수 있다면,"
          line2="조금 더 신뢰할 만한 시스템을 같이 만들어갈 수 있지 않을까."
          size={58}
          line2From={56}
          outAt={137}
          sub="— 그런 막연한 생각에서 시작했습니다."
          subFrom={96}
        />
      </Sequence>
      <Sequence from={1385} durationInFrames={105}>
        <SceneCreed />
      </Sequence>
      <Sequence from={1490} durationInFrames={135}>
        <SceneEnd />
      </Sequence>
    </AbsoluteFill>
  );
};
