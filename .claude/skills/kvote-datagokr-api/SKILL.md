---
name: kvote-datagokr-api
description: kvote의 data.go.kr OpenAPI 계정 연동(api login/list/apply)을 다루는 워크플로. data.go.kr 활용신청·인증키·만료일을 CLI에서 관리하거나, OpenAPI 호출(nec turnout 등)이 인증키를 요구하거나, "활용신청 자동화/신청목록/만료 확인/키 발급" 같은 요청이 나오면 반드시 사용. 로그인 세션을 브라우저에 살려두고 Chrome DevTools Protocol로 다시 붙는 방식(키체인·재로그인·SSO재현 없음)을 안다.
---

# kvote × data.go.kr OpenAPI 계정 연동

data.go.kr 의 OpenAPI 는 **활용신청(자동승인)** 으로 인증키를 받아야 호출할 수 있다.
이 신청은 CAPTCHA + 소셜 로그인 뒤에 있어 완전 자동화가 불가능하다. kvote 는 대신
**브라우저 로그인을 한 번** 받고, 그 세션을 살려둔 채 재사용한다.

## 0. 왜 이렇게 동작하는가 (CDP-attach)

설치형 단일 바이너리를 유지하면서 로그인 세션을 쓰려면 세 가지 함정을 피해야 한다:

- **키체인 프롬프트**: Chrome 127+ 는 쿠키를 app-bound 암호화해 외부 복호화(kooky 류)를
  막는다 → 외부에서 쿠키 DB 읽기는 사양길. 쓰지 마라.
- **세션 쿠키 소멸**: data.go.kr 인증 쿠키(JSESSIONID 등)는 세션 스코프라 **브라우저를
  닫으면 사라진다** → "쿠키 저장 후 재실행"은 깨진다.
- **SSO 핸드셰이크**: 보호 페이지는 `/sso/profile.do` 자동제출 폼을 거친다 → http 로 직접
  재현하면 취약하다.

해법: **브라우저를 데몬으로 살려두고 CDP(원격 디버깅 포트)로 다시 붙는다.** Chrome 이
세션을 네이티브로 들고 SSO 도 알아서 처리하므로, 위 셋이 전부 사라진다. (chrome-cdp-skill /
agent-browser 와 같은 접근. 구현: `internal/datagokr/`.)

## 1. 워크플로

```bash
# (1) 로그인 — 한 번만. 브라우저 창이 뜨면 평소처럼 로그인(네이버 등).
kvote api login          # 로그인 후 그 브라우저는 백그라운드로 유지됨

# (2) 활용신청 현황 — 상태·신청일·만료예정일
kvote api list -f table
kvote api list -f json | jq '.[] | {title, status, expiresAt}'

# (3) 활용신청 — 자동승인 OpenAPI 1건. 목적 필수, 제출 전 확인.
kvote api apply <publicDataPk> --purpose "선거 투표율 분석 연구" --category research
#   --category: research(기본)|web|app|ref|etc   --yes: 확인 생략
#   계정에 실제 신청을 생성하는 동작 → 한 번에 한 건, 목적 필수(§3 참조).

# (4) 세션 종료(선택) — 브라우저 닫고 상태 정리
kvote api logout
```

세션이 만료되면 `api list` 가 `ErrNotLoggedIn` 안내를 낸다 → `kvote api login` 재실행.

## 2. 발급받은 키로 OpenAPI 호출

활용신청이 승인되면 data.go.kr 마이페이지에서 **일반 인증키**(64자 hex)를 받는다. 그 키로
키 기반 명령을 호출한다. **키는 비밀 — 환경변수로만 주고 로그·커밋 금지.**

```bash
export KVOTE_DATAGOKR_KEY=<일반 인증키>
kvote nec turnout 20250603 --sgtype 1     # 투표율(시도/구시군) — OpenAPI
```

## 3. 중립성 + 예의 (반드시 지킬 것)

- **신청은 on-demand 로만**: 작업이 실제로 필요한 API 만 신청한다. "잠재적으로 쓸지도 모르는"
  API 를 투기적으로 대량 신청하지 마라 — 활용목적 명시 취지 위반이고 계정이 플래그될 수 있다.
  data.go.kr 은 신청 시 활용목적을 적게 돼 있다. 그 목적에 맞게만.
- **중립성**: kvote 는 데이터 접근성만 높인다. 받은 데이터로 정상/비정상/조작을 단정하지
  않는다(→ [[kvote-verify]] 와 동일 원칙). 키/세션은 수단일 뿐.
- **rate limit·예의**: 불필요한 반복 호출 금지. 세션 브라우저는 사용자의 실제 계정이다.

## 4. 스키마

`kvote api list` 항목(`Application`):
`{title, status(승인/신청), org, category, account(개발/운영), appliedAt, expiresAt, uddi, detailPk}`
— `expiresAt`(만료예정일)이 갱신 추적의 핵심 필드.

## 5. 트러블슈팅

- **로그인 창에서 로그인이 안 끝남**: `api login` 은 인증 페이지가 실제로 열릴 때까지 기다린다.
  열린 창에서 로그인을 *완료*해야 감지된다(URL 이 포털로 복귀). 창이 다른 창 뒤에 있을 수 있다.
- **`ErrNotLoggedIn`**: 세션 브라우저가 닫혔거나 만료 → `kvote api login` 재실행.
- **Chrome 못 찾음**: Chrome/Chromium/Brave/Edge 중 하나가 설치돼 있어야 한다.
- **키 401**: 활용신청 직후 키는 ~1시간 전파 지연이 있을 수 있다.
