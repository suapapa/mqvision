# MQVision

<p align="center">
  <img src="_img/logo.webp" alt="mqvision_logo" width="220">
</p>

MQVision은 [esp32_cam2mqtt](https://github.com/suapapa/esp32_cam2mqtt)로 이미지를 받아
LLM으로 가스 미터 값을 읽어,
원본 이미지는 보관하며, 웹서버로 제공하는 프로그램입니다.

> HomeAssistant RESTful integration과 맞춰 두었습니다.

## 주요 기능

- **MQTT 이미지 수신**: MQTT 토픽에서 센서 이미지를 실시간으로 받음
- **AI 센서값 추출**: Google Gemini로 가스 미터 이미지에서 값을 읽음
- **이미지 보관**: 받은 원본을 [Concierge 서비스](https://github.com/suapapa/concierge)에 저장
- **RESTful API**: 읽은 센서값을 웹으로 제공해 HomeAssistant와 연동

## 요구사항

- MQTT 브로커 접근 권한
- Google Gemini API 키
- Concierge 서비스 (이미지 저장용)

## 설치

```bash
git clone <repository-url>
cd gas-meter-reader
go mod download
go build -o mqvision
```

## 설정

1. `.env.example`을 복사해 `.env`를 만들고 값을 채웁니다:

```bash
cp .env.example .env
```

```env
MQTT_HOST=mqtt://username:password@mqtt-broker-address
MQTT_TOPIC=your-topic/gas-meter/cam
CONCIERGE_ADDR=http://concierge-service-address
CONCIERGE_TOKEN=concierge-token
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-proj-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
OPENAI_MODEL=gpt-4o-mini
```

2. 환경 변수 설명:
   - `MQTT_HOST`: MQTT 브로커 연결 정보 (형식: `mqtt://username:password@host:port`)
   - `MQTT_TOPIC`: 센서 이미지를 받을 MQTT 토픽
   - `CONCIERGE_ADDR`: 이미지를 저장할 Concierge 서비스 주소
   - `CONCIERGE_TOKEN`: Concierge 서비스 인증 토큰
   - `OPENAI_BASE_URL`: OpenAI 호환 API base URL
   - `OPENAI_API_KEY`: OpenAI 호환 API 키
   - `OPENAI_MODEL`: 사용할 비전 모델

3. `prompt.yaml`에는 프롬프트만 둡니다 (저장소에 포함됨):

```yaml
read_gas_gauge:
  system: |
    [가스 미터 이미지 분석 시스템 프롬프트]
  user: |
    [가스 미터 이미지 분석 유저 프롬프트]

fix_ambiguous:
  system: |
    [모호한 자릿수 보정 시스템 프롬프트]
  user: |
    Ambiguous reading: {{ambiguous}}
    Previous reading: {{previous}}
```

`fix_ambiguous.user`의 `{{ambiguous}}`, `{{previous}}`는 실행 시 실제 값으로 바뀝니다.

## 사용 방법

### 일반 실행 (MQTT 모드)

```bash
./mqvision -p 8080 -c prompt.yaml
```

- `-p`: 웹서버 포트 (기본값: 8080)
- `-c`: 설정 파일 경로 (기본값: prompt.yaml)

브라우저에서 `http://localhost:8080/` 로 모니터링 대시보드에 들어갈 수 있습니다.
(프론트엔드를 빌드해 `web/dist`가 있어야 합니다.)

### 프론트엔드 개발

```bash
# 터미널 1: API 서버
./mqvision -p 8080 -c prompt.yaml

# 터미널 2: Vite 개발 서버 (API는 localhost:8080으로 프록시)
cd web && npm install && npm run dev
```

프로덕션용 정적 빌드:

```bash
cd web && npm run build
# web/dist 를 Gin이 / 로 서빙
```

## API 엔드포인트

### GET /api/sensor

최신 센서값을 반환합니다.

**응답 예시:**

```json
{
  "value": 2924.457,
  "updated_at": "2025-11-07T05:13:17+09:00",
  "metadata": {
    "read": "02924.457",
    "read_at": "2025-11-07T05:13:17+09:00",
    "it_takes": "2.5s",
    "src_image_url": "http://concierge-service/image-url"
  }
}
```

**에러 응답 (값이 아직 없는 경우):**

```json
{
  "error": "no value yet"
}
```

### GET /api/health

MQTT 연결 상태와 앱 헬스 체크 정보를 반환합니다.

**성공 시 응답 예시 (HTTP 200):**

```json
{
  "status": "ok",
  "mqtt": {
    "connected": true,
    "last_error": null
  },
  "sensor": {
    "last_updated": "2025-11-07T05:13:17+09:00"
  }
}
```

**오류 시 응답 예시 (HTTP 503):**

```json
{
  "status": "fail",
  "mqtt": {
    "connected": false,
    "last_error": "network Error: connection refused"
  },
  "sensor": {
    "last_updated": null
  }
}
```

Docker 이미지와 `docker-compose.yml`은 이 엔드포인트를 healthcheck로 씁니다. MQTT가 끊기면 unhealthy가 됩니다. Compose의 `restart: unless-stopped`만으로는 unhealthy일 때 재시작되지 않으니, MQTT가 약 2분 이상 끊기면 프로세스가 exit 1로 죽고 컨테이너가 다시 뜹니다. 재연결되면 OnConnect에서 토픽을 다시 Subscribe합니다.

## HomeAssistant 연동

HomeAssistant [RESTful Sensor](https://www.home-assistant.io/integrations/sensor.rest)로
센서값을 연동할 수 있습니다.

`configuration.yaml`에 다음을 추가:

```yaml
sensor:
  - platform: rest
    name: Gas Meter Reading
    resource: http://mqvision-server:8080/api/sensor
    value_template: "{{ value_json.value }}"
    unit_of_measurement: "m³"
    json_attributes:
      - updated_at
      - metadata
    scan_interval: 300  # 5분마다 업데이트
```

## 동작 흐름

1. MQTT 토픽에서 센서 이미지 수신
2. 받은 이미지를 두 갈래로 나눔:
   - Concierge로 보내 원본 저장
   - Gemini로 보내 센서값 추출
3. 추출한 센서값을 내부 상태에 저장
4. 웹서버가 최신 센서값 제공

### HomeAssistant 통합 결과

![ha](_img/ha.jpeg)
