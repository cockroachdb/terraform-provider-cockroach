// Copyright 2023 The Cockroach Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.
// CockroachDB Cloud API
// API version: 2023-04-10

package client

import (
	"encoding/json"
	"time"
)

// Invoice Invoice message represents the details and the total charges associated with one billing period, which starts at the beginning of the month and ends at the beginning of the next month.  The message also includes details about each invoice item..
type Invoice struct {
	// adjustments is a list of credits or costs that adjust the value of the invoice (e.g. a Serverless Free Credit or Premium Support adjustment). Unlike line items, adjustments are not tied to a particular cluster.
	Adjustments *[]InvoiceAdjustment `json:"adjustments,omitempty"`
	// balances are the amounts of currency left at the time of the invoice.
	Balances []CurrencyAmount `json:"balances"`
	// invoice_id is the unique ID representing the invoice.
	InvoiceId string `json:"invoice_id"`
	// invoice_items are sorted by the cluster name.
	InvoiceItems []InvoiceItem `json:"invoice_items"`
	// period_end is the end of the billing period (exclusive).
	PeriodEnd time.Time `json:"period_end"`
	// period_start is the start of the billing period (inclusive).
	PeriodStart time.Time `json:"period_start"`
	// totals is a list of the total amounts per currency.
	Totals []CurrencyAmount `json:"totals"`
}

// NewInvoice instantiates a new Invoice object.
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewInvoice(balances []CurrencyAmount, invoiceId string, invoiceItems []InvoiceItem, periodEnd time.Time, periodStart time.Time, totals []CurrencyAmount) *Invoice {
	p := Invoice{}
	p.Balances = balances
	p.InvoiceId = invoiceId
	p.InvoiceItems = invoiceItems
	p.PeriodEnd = periodEnd
	p.PeriodStart = periodStart
	p.Totals = totals
	return &p
}

// NewInvoiceWithDefaults instantiates a new Invoice object.
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewInvoiceWithDefaults() *Invoice {
	p := Invoice{}
	return &p
}

// GetAdjustments returns the Adjustments field value if set, zero value otherwise.
func (o *Invoice) GetAdjustments() []InvoiceAdjustment {
	if o == nil || o.Adjustments == nil {
		var ret []InvoiceAdjustment
		return ret
	}
	return *o.Adjustments
}

// SetAdjustments gets a reference to the given []InvoiceAdjustment and assigns it to the Adjustments field.
func (o *Invoice) SetAdjustments(v []InvoiceAdjustment) {
	o.Adjustments = &v
}

// GetBalances returns the Balances field value.
func (o *Invoice) GetBalances() []CurrencyAmount {
	if o == nil {
		var ret []CurrencyAmount
		return ret
	}

	return o.Balances
}

// SetBalances sets field value.
func (o *Invoice) SetBalances(v []CurrencyAmount) {
	o.Balances = v
}

// GetInvoiceId returns the InvoiceId field value.
func (o *Invoice) GetInvoiceId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.InvoiceId
}

// SetInvoiceId sets field value.
func (o *Invoice) SetInvoiceId(v string) {
	o.InvoiceId = v
}

// GetInvoiceItems returns the InvoiceItems field value.
func (o *Invoice) GetInvoiceItems() []InvoiceItem {
	if o == nil {
		var ret []InvoiceItem
		return ret
	}

	return o.InvoiceItems
}

// SetInvoiceItems sets field value.
func (o *Invoice) SetInvoiceItems(v []InvoiceItem) {
	o.InvoiceItems = v
}

// GetPeriodEnd returns the PeriodEnd field value.
func (o *Invoice) GetPeriodEnd() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.PeriodEnd
}

// SetPeriodEnd sets field value.
func (o *Invoice) SetPeriodEnd(v time.Time) {
	o.PeriodEnd = v
}

// GetPeriodStart returns the PeriodStart field value.
func (o *Invoice) GetPeriodStart() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.PeriodStart
}

// SetPeriodStart sets field value.
func (o *Invoice) SetPeriodStart(v time.Time) {
	o.PeriodStart = v
}

// GetTotals returns the Totals field value.
func (o *Invoice) GetTotals() []CurrencyAmount {
	if o == nil {
		var ret []CurrencyAmount
		return ret
	}

	return o.Totals
}

// SetTotals sets field value.
func (o *Invoice) SetTotals(v []CurrencyAmount) {
	o.Totals = v
}

func (o Invoice) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if o.Adjustments != nil {
		toSerialize["adjustments"] = o.Adjustments
	}
	if true {
		toSerialize["balances"] = o.Balances
	}
	if true {
		toSerialize["invoice_id"] = o.InvoiceId
	}
	if true {
		toSerialize["invoice_items"] = o.InvoiceItems
	}
	if true {
		toSerialize["period_end"] = o.PeriodEnd
	}
	if true {
		toSerialize["period_start"] = o.PeriodStart
	}
	if true {
		toSerialize["totals"] = o.Totals
	}
	return json.Marshal(toSerialize)
}
