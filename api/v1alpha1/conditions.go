package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *APK) GetConditions() []metav1.Condition {
	return a.Status.Conditions
}

func (a *APK) SetConditions(conditions []metav1.Condition) {
	a.Status.Conditions = conditions
}

func (b *Bucket) GetConditions() []metav1.Condition {
	return b.Status.Conditions
}

func (b *Bucket) SetConditions(conditions []metav1.Condition) {
	b.Status.Conditions = conditions
}

func (i *IPA) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *IPA) SetConditions(conditions []metav1.Condition) {
	i.Status.Conditions = conditions
}

func (m *MobileApp) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

func (m *MobileApp) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}
