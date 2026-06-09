import { ComponentFixture, TestBed } from '@angular/core/testing';

import { Tiendas } from './tiendas';

describe('Tiendas', () => {
  let component: Tiendas;
  let fixture: ComponentFixture<Tiendas>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Tiendas],
    }).compileComponents();

    fixture = TestBed.createComponent(Tiendas);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
