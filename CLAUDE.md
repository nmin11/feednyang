# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Discord bot for tech blog feeds built using AWS infrastructure managed by Pulumi. The project uses TypeScript with Pulumi for infrastructure as code and is designed to deploy AWS Lambda functions for Discord bot functionality.

**Key Feature**: 유명 빅테크 기업들의 새로운 기술 블로그 피드를 디스코드 봇을 통해 디스코드 채널에 전파한다.

- [아키텍처 다이어그램](./docs/architecture-diagram.svg)
- [MongoDB 스키마](./docs/mongodb-schema.md)

## Tech Stack

- **Languages**: TypeScript (Pulumi), Go (Lambda functions)
- **Infrastructure Management**: Pulumi with TypeScript for AWS resource provisioning
- **Package Manager**: Bun as specified in Pulumi.yaml configuration
- **Runtime**: Node.js with TypeScript
- **Target Platform**: AWS Lambda for serverless execution
- **Database**: MongoDB (중복된 기술 블로그 피드 확인 용도)

### AWS Infrastructure

- **EventBridge**: 한국 시간 기준으로 평일 08, 12, 18, 22시, 토요일 12시에 Lambda 함수 호출
- **Lambda**: 기술 블로그의 RSS 피드를 모아오고, 중복 피드를 제거한 작업을 거친 후에, 디스코드 봇에게 메시지 전파를 요청

## Common Commands

### Pulumi Operations

```bash
# Preview infrastructure changes
pulumi preview

# Deploy infrastructure
pulumi up

# Destroy infrastructure
pulumi destroy

# Remove stack completely
pulumi stack rm
```

### Package Management (using Bun)

```bash
# Install dependencies
bun install

# Add new dependency
bun add <package-name>

# Add dev dependency
bun add -d <package-name>
```

### TypeScript

```bash
# Compile TypeScript
npx tsc

# Type checking
npx tsc --noEmit
```

## Configuration

Pulumi configuration is managed through:

- `Pulumi.yaml`: Project metadata and runtime configuration
- `Pulumi.dev.yaml`: Stack-specific configuration
- Use `pulumi config set <key> <value>` for configuration management

AWS credentials must be configured via AWS CLI or environment variables before deployment.

## Development Structure

When adding Lambda functions or other AWS resources:

1. Create separate directories for Lambda function code (e.g., `lambda/function-name.go`)
2. Define all infrastructure resources in `index.ts` (main entry point)
3. Use Pulumi's asset and archive capabilities to package Lambda functions
4. Export important resource identifiers for reference

The project follows Pulumi's infrastructure-as-code patterns with TypeScript for type safety and developer experience.

## Convention & Rules

- 시크릿 값은 코드 및 설정 파일에서 유출하지 않도록 한다.
- `pulumi` 명령어는 사용자가 직접 터미널에 입력해 확인할 수 있도록 한다.
- Indent
  - TypeScript: 2
  - Go: Tabs 간격
- JSON 표현 시 의미 없는 마지막 쉼표(`,`) 생략
