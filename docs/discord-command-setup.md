# Discord Slash Command 등록 방법

Discord 봇의 슬래시 커맨드는 **글로벌 커맨드**로 등록되어 한 번만 설정하면 모든 서버에서 사용할 수 있습니다.

## 커맨드 목록

- `/add <url>` - 새로운 RSS 피드 추가
- `/remove <identifier>` - 피드 삭제 (번호, 이름, URL로 식별)
- `/list` - 등록된 피드 목록 조회
- `/help` - 봇 사용법 및 명령어 도움말

## 등록 방법

### 1. 환경 변수 설정

```bash
export DISCORD_BOT_TOKEN="your_bot_token"
export DISCORD_APP_ID="your_application_id"
```

### 2. curl로 직접 등록

```bash
# /add 커맨드
curl -X POST \
  "https://discord.com/api/v10/applications/$DISCORD_APP_ID/commands" \
  -H "Authorization: Bot $DISCORD_BOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "add",
    "description": "새로운 RSS 피드 추가",
    "type": 1,
    "options": [{
      "type": 3,
      "name": "url",
      "description": "추가할 RSS 피드 URL",
      "required": true
    }]
  }'

# /remove 커맨드
curl -X POST \
  "https://discord.com/api/v10/applications/$DISCORD_APP_ID/commands" \
  -H "Authorization: Bot $DISCORD_BOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "remove",
    "description": "등록된 RSS 피드 삭제",
    "type": 1,
    "options": [{
      "type": 3,
      "name": "identifier",
      "description": "삭제할 피드 (번호, 이름, URL)",
      "required": true
    }]
  }'

# /list 커맨드
curl -X POST \
  "https://discord.com/api/v10/applications/$DISCORD_APP_ID/commands" \
  -H "Authorization: Bot $DISCORD_BOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "list",
    "description": "등록된 RSS 피드 목록 조회",
    "type": 1
  }'

# /help 커맨드
curl -X POST \
  "https://discord.com/api/v10/applications/$DISCORD_APP_ID/commands" \
  -H "Authorization: Bot $DISCORD_BOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "help",
    "description": "봇 사용법 및 명령어 도움말",
    "type": 1
  }'
```

## 참고사항

- 글로벌 커맨드는 등록 후 최대 1시간까지 반영 시간이 걸릴 수 있습니다
- 테스트 환경에서는 길드 커맨드 사용을 권장합니다 (즉시 반영)
- 커맨드 수정 시에는 기존 커맨드를 DELETE 후 새로 등록하세요
