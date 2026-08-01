from enum import StrEnum


class FinalStatus(StrEnum):
    PASS = "PASS"
    FAIL = "FAIL"
    BLOCKED = "BLOCKED"
    INCONCLUSIVE = "INCONCLUSIVE"


class CriterionStatus(StrEnum):
    PASS = "PASS"
    FAIL = "FAIL"
    UNKNOWN = "UNKNOWN"
    NOT_APPLICABLE = "N/A"


EXIT_CODES = {
    FinalStatus.PASS: 0,
    FinalStatus.FAIL: 1,
    FinalStatus.BLOCKED: 2,
    FinalStatus.INCONCLUSIVE: 3,
}
