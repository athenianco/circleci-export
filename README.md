# circleci-export

Load pipelines from CircleCI and submit them to Athenian as releases.

## Installation

```
GOBIN=$(pwd) go install github.com/athenianco/circleci-labeler@latest
```

## Usage

Obtain a [CircleCI Personal API token](https://circleci.com/docs/2.0/managing-api-tokens/).
Obtain an [Athenian API token](https://api.athenian.co/v1/ui/#/security/create_token).

Given a GitHub repository `org/repo` with the main `branch`,

```
export CIRCLECI_TOKEN=...
export ATHENIAN_TOKEN=...
./circleci-export org/repo@branch
```

If `@branch` is not added, submit all the branches.

## License

Apache 2.0, see [LICENSE](LICENSE).