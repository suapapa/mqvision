# MQVision

![mqvision_logo](_img/logo.png)

MQVision은 [esp32_cam2mqtt](https://github.com/suapapa/esp32_cam2mqtt) 디바이스를 통해
MQTT 토픽에서 센서 이미지를 수신하여 Genkit(Gemini AI)을 사용해 가스 미터 값을 자동으로 읽고,
원본 이미지를 보관하며, 읽은 센서값을 웹서버로 제공하는 프로그램입니다.

> HomeAssistant의 RESTful integration과 함께 사용하기 위해 설계되었습니다.

## 주요 기능

- **MQTT 이미지 수신**: MQTT 토픽에서 센서 이미지를 실시간으로 수신
- **AI 기반 센서값 추출**: Google Gemini AI를 사용하여 가스 미터 이미지에서 자동으로 값을 읽음
- **이미지 보관**: 수신한 원본 이미지를 [Concierge 서비스](https://github.com/suapapa/concierge)를 통해 저장
- **RESTful API**: 읽은 센서값을 웹서버를 통해 제공하여 HomeAssistant와 연동 가능

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
   - `MQTT_TOPIC`: 센서 이미지를 수신할 MQTT 토픽
   - `CONCIERGE_ADDR`: 이미지를 저장할 Concierge 서비스 주소
   - `CONCIERGE_TOKEN`: Concierge 서비스 인증 토큰
   - `OPENAI_BASE_URL`: OpenAI 호환 API base URL
   - `OPENAI_API_KEY`: OpenAI 호환 API 키
   - `OPENAI_MODEL`: 사용할 비전 모델

3. `config.yaml`에는 프롬프트만 둡니다 (저장소에 포함됨):

```yaml
prompt:
  system: |
    [시스템 프롬프트 내용]
  user: |
    [유저 프롬프트 내용]
```

## 사용 방법

### 일반 실행 (MQTT 모드)

```bash
./mqvision -p 8080 -c config.yaml
```

- `-p`: 웹서버 포트 (기본값: 8080)
- `-c`: 설정 파일 경로 (기본값: config.yaml)

브라우저에서 `http://localhost:8080/` 로 모니터링 대시보드에 접근할 수 있습니다.
(프론트엔드를 빌드해 `web/dist`가 있어야 합니다.)

### 프론트엔드 개발

```bash
# 터미널 1: API 서버
./mqvision -p 8080 -c config.yaml

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

MQTT 연결 상태 및 애플리케이션 헬스 체크 정보를 반환합니다.

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

Docker 이미지/`docker-compose.yml`은 이 엔드포인트를 healthcheck로 사용합니다 (MQTT 미연결 시 unhealthy). Compose의 `restart: unless-stopped`만으로는 unhealthy 시 재시작되지 않으므로, MQTT가 약 2분 이상 끊기면 프로세스가 exit 1로 종료되어 컨테이너가 다시 뜹니다. 재연결 시에는 OnConnect에서 토픽을 다시 Subscribe합니다.

## HomeAssistant 연동

HomeAssistant의 [RESTful Sensor](https://www.home-assistant.io/integrations/sensor.rest)를
사용하여 센서값을 연동할 수 있습니다.

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
2. 수신한 이미지를 두 개의 파이프로 분기:
   - Concierge 서비스로 전송하여 원본 이미지 저장
   - Gemini AI로 전송하여 센서값 추출
3. 추출된 센서값을 내부 상태에 저장
4. 웹서버를 통해 최신 센서값 제공

### HomeAssistant 통합 결과

![ha](_img/ha.jpeg)
