# MQVision

![mqvision_logo](_img/logo.png)

MQVision은 ![esp32_cam2mqtt](https://github.com/suapapa/esp32_cam2mqtt) 디바이스를 통해
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

1. `config_example.yaml`을 참고하여 `config.yaml` 파일을 생성합니다:

```yaml
mqtt:
  host: mqtt://username:password@mqtt-broker-address
  topic: your-topic/gas-meter/cam
concierge:
  addr: http://concierge-service-address
  token: concierge-token
gemini:
  api_key: your-gemini-api-key
  model: googleai/gemini-2.5-flash-lite
  system_prompt: |
    [시스템 프롬프트 내용]
  prompt: |
    [프롬프트 내용]
```

2. 각 설정 항목 설명:
   - `mqtt.host`: MQTT 브로커 연결 정보 (형식: `mqtt://username:password@host:port`)
   - `mqtt.topic`: 센서 이미지를 수신할 MQTT 토픽
   - `concierge.addr`: 이미지를 저장할 Concierge 서비스 주소
   - `concierge.token`: Concierge 서비스 인증 토큰
   - `gemini.api_key`: Google Gemini API 키
   - `gemini.model`: 사용할 Gemini 모델
   - `gemini.system_prompt`: AI에게 전달할 시스템 프롬프트
   - `gemini.prompt`: AI에게 전달할 프롬프트

## 사용 방법

### 일반 실행 (MQTT 모드)

```bash
./mqvision -p 8080 -c config.yaml
```

- `-p`: 웹서버 포트 (기본값: 8080)
- `-c`: 설정 파일 경로 (기본값: config.yaml)

## API 엔드포인트

### GET /sensor

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

## HomeAssistant 연동

HomeAssistant의 [RESTful Sensor](https://www.home-assistant.io/integrations/sensor.rest)를
사용하여 센서값을 연동할 수 있습니다.

`configuration.yaml`에 다음을 추가:

```yaml
sensor:
  - platform: rest
    name: Gas Meter Reading
    resource: http://mqvision-server:8080/sensor
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
