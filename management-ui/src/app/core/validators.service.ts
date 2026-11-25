import { Injectable } from '@angular/core';
import {
    AbstractControl,
    FormControl,
    FormGroup,
    ValidatorFn,
} from '@angular/forms';
import { Patterns } from '../shared/validator-patterns';

@Injectable({
    providedIn: 'root',
})
export class ValidatorsService {
    constructor() {}

    getIpv4v6CidrValidator(): ValidatorFn {
        return (formControl: FormControl): { [key: string]: any } => {
            const input = formControl.value;

            if (!input) {
                return null;
            }
            if (
                !Patterns.IPv4.test(input) &&
                !Patterns.IPv6.test(input)
            ) {
                return { ipv4v6Cidr: true };
            }
            return null;
        };
    }

    getIpv4v6CidrRequiredValidator(): ValidatorFn {
        return (formControl: FormControl): { [key: string]: any } => {
            const input = formControl.value;

            if (!input) {
                return null;
            }
            if (
                !Patterns.IPv4CIDR.test(input) &&
                !Patterns.IPv6CIDR.test(input)
            ) {
                return { ipv4v6CidrRequired: true };
            }
            return null;
        };
    }

    getGroupRequirementValidator(
        form: FormGroup,
        groupMembers: string[]
    ): ValidatorFn {
        return (control: AbstractControl) => {
            const someMemberFilled = groupMembers.some(
                (memberName) =>
                    form &&
                    form.get(memberName) &&
                    form.get(memberName).value &&
                    form.get(memberName).value.length > 0
            );
            return someMemberFilled &&
                (!control.value || control.value.length === 0)
                ? { groupRequired: { value: true } }
                : null;
        };
    }
}
